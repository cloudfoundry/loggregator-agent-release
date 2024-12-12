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
)

type HTTPSBatchWriter struct {
	HTTPSWriter
	batchSize    int
	sendInterval time.Duration
	msgChan      chan []byte
	quit         chan struct{}
	wg           sync.WaitGroup
}

type Option func(*HTTPSBatchWriter)

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

func NewHTTPSBatchWriter(
	binding *URLBinding,
	netConf NetworkTimeoutConfig,
	tlsConf *tls.Config,
	egressMetric metrics.Counter,
	c *Converter,
	options ...Option,
) egress.WriteCloser {
	client := httpClient(netConf, tlsConf)
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
		batchSize:    256 * 1024,        // Default value
		sendInterval: 1 * time.Second,   // Default value
		msgChan:      make(chan []byte), // Buffered channel for messages
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
			w.sendHttpRequest(msgBatch.Bytes(), msgCount) // nolint:errcheck
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
	w.wg.Wait() // Ensure sender finishes processing before closing
	close(w.msgChan)
	return nil
}
