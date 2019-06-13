package syslog

import (
	"log"
	"math"
	"time"

	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	"code.cloudfoundry.org/loggregator-agent/pkg/egress"
)

// RetryWrapper wraps a WriterConstructer, allowing it to retry writes.
func RetryWrapper(
	wc WriterConstructor,
	r RetryDuration,
	maxRetries int,
	logClient LogClient,
	sourceIndex string,
) WriterConstructor {
	return WriterConstructor(func(
		binding *URLBinding,
		netConf NetworkTimeoutConfig,
		skipCertVerify bool,
		egressMetric func(uint64),
	) egress.WriteCloser {
		writer := wc(
			binding,
			netConf,
			skipCertVerify,
			egressMetric,
		)

		return &RetryWriter{
			writer:        writer,
			retryDuration: r,
			maxRetries:    maxRetries,
			binding:       binding,
			logClient:     logClient,
			sourceIndex:   sourceIndex,
		}
	})
}

// RetryDuration calculates a duration based on the number of write attempts.
type RetryDuration func(attempt int) time.Duration

// RetryWriter wraps a WriteCloser and will retry writes if the first fails.
type RetryWriter struct {
	writer        egress.WriteCloser
	retryDuration RetryDuration
	maxRetries    int
	binding       *URLBinding
	logClient     LogClient
	sourceIndex   string
}

// Write will retry writes unitl maxRetries has been reached.
func (r *RetryWriter) Write(e *loggregator_v2.Envelope) error {
	logTemplate := "failed to write to %s, retrying in %s, err: %s"

	var err error

	for i := 0; i < r.maxRetries; i++ {
		err = r.writer.Write(e)
		if err == nil {
			return nil
		}

		if egress.ContextDone(r.binding.Context) {
			return err
		}

		sleepDuration := r.retryDuration(i)
		log.Printf(logTemplate, r.binding.URL.Host, sleepDuration, err)

		time.Sleep(sleepDuration)
	}

	return err
}

// Close delegates to the syslog writer.
func (r *RetryWriter) Close() error {
	return r.writer.Close()
}

// ExponentialDuration returns a duration that grows exponentially with each
// attempt. It is maxed out at 15 seconds.
func ExponentialDuration(attempt int) time.Duration {
	if attempt == 0 {
		return time.Millisecond
	}

	tenthDuration := int(math.Pow(2, float64(attempt-1)) * 100)
	duration := time.Duration(tenthDuration*10) * time.Microsecond

	if duration > 15*time.Second {
		return 15 * time.Second
	}

	return duration
}
