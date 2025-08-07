package syslog

import (
	"bytes"
	"crypto/tls"
	"log"
	"sync"
	"time"

	"code.cloudfoundry.org/go-loggregator/v10/rpc/loggregator_v2"
	metrics "code.cloudfoundry.org/go-metric-registry"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress"
	"github.com/valyala/fasthttp"
)

// --- Coordinator definition ---
type RetryCoordinator struct {
	sem chan struct{}
}

var (
	globalRetryCoordinator     *RetryCoordinator
	globalRetryCoordinatorOnce sync.Once
	maxParallelRetries         = 4
)

// testing override for maxParallelRetries
func WithParallelRetries(n int) {
	globalRetryCoordinator = &RetryCoordinator{
		sem: make(chan struct{}, n),
	}
}

func GetGlobalRetryCoordinator() *RetryCoordinator {
	globalRetryCoordinatorOnce.Do(func() {
		globalRetryCoordinator = &RetryCoordinator{
			sem: make(chan struct{}, maxParallelRetries),
		}
	})
	return globalRetryCoordinator
}

func (c *RetryCoordinator) Acquire(URL string) {
	select {
	case c.sem <- struct{}{}:
		return
	default:
		log.Printf("All retry slots (%d) are in use. Log delivery for %s may be delayed.",
			maxParallelRetries, URL)
		c.sem <- struct{}{}
	}
}

func (c *RetryCoordinator) Release() {
	<-c.sem
}

type InternalRetryWriter interface {
	ConfigureRetry(retryDuration RetryDuration, maxRetries int)
}

type Retryer struct {
	retryDuration RetryDuration
	maxRetries    int
	binding       *URLBinding
	coordinator   *RetryCoordinator
}

func NewRetryer(
	binding *URLBinding,
	retryDuration RetryDuration,
	maxRetries int,
) *Retryer {
	return &Retryer{
		retryDuration: retryDuration,
		maxRetries:    maxRetries,
		binding:       binding,
		coordinator:   GetGlobalRetryCoordinator(),
	}
}

// Retry will retry the provided function up to maxRetries times.
// It returns true if the function failed after all retries, false otherwise.
func (r *Retryer) Retry(batch []byte, msgCount float64, funcToRetry func([]byte, float64) error) (failed bool) {
	var err error

	// First attempt (fast path, not counted as a retry)
	err = funcToRetry(batch, msgCount)
	if err == nil {
		return false
	}

	if egress.ContextDone(r.binding.Context) {
		log.Printf("Context cancelled for %s, aborting retries", r.binding.URL.Host)
		return true
	}

	log.Printf("Failed to write to %s, retrying in %s, err: %s", r.binding.URL.Host, r.retryDuration(0), err)

	for i := 0; i < r.maxRetries-1; i++ {

		if egress.ContextDone(r.binding.Context) {
			log.Printf("Context cancelled for %s, aborting retries", r.binding.URL.Host)
			return true
		}

		sleepDuration := r.retryDuration(i)
		time.Sleep(sleepDuration)

		r.coordinator.Acquire(r.binding.URL.Host)
		func() {
			defer r.coordinator.Release()
			err = funcToRetry(batch, msgCount)
		}()
		if err == nil {
			return false
		}
		log.Printf("Failed to write to %s, retrying in %s, err: %s", r.binding.URL.Host, r.retryDuration(i+1), err)

	}

	log.Printf("Exhausted retries for %s, dropping batch with %.0f messages, err: %s",
		r.binding.URL.Host, msgCount, err)
	return true
}

type HTTPSBatchWriter struct {
	HTTPSWriter
	batchSize    int
	sendInterval time.Duration
	retryer      Retryer
	msgChan      chan []byte
	quit         chan struct{}
	wg           sync.WaitGroup
}

// Also Marks that HTTPSBatchWriter implements the InternalRetryWriter interface
func (w *HTTPSBatchWriter) ConfigureRetry(retryDuration RetryDuration, maxRetries int) {
	w.retryer.retryDuration = retryDuration
	w.retryer.maxRetries = maxRetries
}

type Option func(*HTTPSBatchWriter)

// testing override for batch size and send interval
func WithBatchSize(size int) Option {
	return func(w *HTTPSBatchWriter) {
		w.batchSize = size
	}
}
func WithSendInterval(interval time.Duration) Option {
	return func(w *HTTPSBatchWriter) {
		w.sendInterval = interval
	}
}

// --- HTTPSBatchWriter definition ---

// HTTPSBatchWriter is an egress.WriteCloser implementation that batches syslog messages
// and sends them via HTTPS in configurable batch sizes and intervals. It provides
// backpressure to upstream callers by using a blocking channel for incoming messages.
// Failed batch sends are retried according to a configurable retry policy, using a
// global RetryCoordinator to limit the number of concurrent retries across all drains.
// This prevents resource exhaustion and noisy neighbor issues, ensuring reliable and
// efficient delivery of batched syslog messages.
func NewHTTPSBatchWriter(
	binding *URLBinding,
	netConf NetworkTimeoutConfig,
	tlsConf *tls.Config,
	egressMetric metrics.Counter,
	c *Converter,
	options ...Option,
) egress.WriteCloser {
	client := httpBatchClient(netConf, tlsConf)
	binding.URL.Scheme = "https"

	writer := &HTTPSBatchWriter{
		HTTPSWriter: HTTPSWriter{
			url:             binding.URL,
			appID:           binding.AppID,
			hostname:        binding.Hostname,
			client:          client,
			egressMetric:    egressMetric,
			syslogConverter: c,
		},
		retryer:      *NewRetryer(binding, ExponentialDuration, 0), // Will be set by ConfigureRetry later
		batchSize:    512 * 1024,                                   // Default value
		sendInterval: 1 * time.Second,                              // Default value
		msgChan:      make(chan []byte),                            // blocking single message channel for backpressure
		quit:         make(chan struct{}),
	}

	for _, opt := range options {
		opt(writer)
	}

	writer.wg.Add(1)
	go writer.startSender()

	return writer
}

func (w *HTTPSBatchWriter) Write(env *loggregator_v2.Envelope) error {
	msgs, err := w.syslogConverter.ToRFC5424(env, w.hostname)
	if err != nil {
		log.Printf("Failed to parse syslog, dropping message, err: %s", err)
		return nil
	}

	for _, msg := range msgs {
		w.msgChan <- msg
	}

	return nil
}

func (w *HTTPSBatchWriter) startSender() {
	defer w.wg.Done()

	ticker := time.NewTicker(w.sendInterval)
	defer ticker.Stop()

	var msgBatch bytes.Buffer
	var msgCount float64

	sendBatch := func() {
		if msgBatch.Len() > 0 {
			failed := w.retryer.Retry(msgBatch.Bytes(), msgCount, w.sendHttpRequest)
			if failed {
				log.Printf("Failed to deliver %.0f messages to %s after all retries, dropping batch",
					msgCount, w.url.Host)
			}
			msgBatch.Reset()
			msgCount = 0
		}
	}

	for {
		select {
		case msg := <-w.msgChan:
			_, err := msgBatch.Write(msg)
			if err != nil {
				log.Printf("Failed to write to buffer, dropping buffer of size %d , err: %s", msgBatch.Len(), err)
				msgBatch.Reset()
				msgCount = 0
			} else {
				msgCount++
				if msgBatch.Len() >= w.batchSize {
					sendBatch()
				}
			}
		case <-ticker.C:
			sendBatch()
		case <-w.quit:
			sendBatch()
			return
		}
	}
}

func (w *HTTPSBatchWriter) Close() error {
	close(w.quit)
	w.wg.Wait()
	return nil
}

func httpBatchClient(netConf NetworkTimeoutConfig, tlsConf *tls.Config) *fasthttp.Client {
	client := httpClient(netConf, tlsConf)
	client.MaxIdleConnDuration = 30 * time.Second
	client.MaxConnDuration = 30 * time.Second
	return client
}
