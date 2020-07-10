package syslog

import (
	"log"
	"math"
	"time"

	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress"
)

// maxRetries for the backoff, results in around an hour of total delay
const maxRetries int = 22

// RetryDuration calculates a duration based on the number of write attempts.
type RetryDuration func(attempt int) time.Duration

// RetryWriter wraps a WriteCloser and will retry writes if the first fails.
type RetryWriter struct {
	Writer        egress.WriteCloser //public to allow testing
	retryDuration RetryDuration
	maxRetries    int
	binding       *URLBinding
}

func NewRetryWriter(
	urlBinding *URLBinding,
	retryDuration RetryDuration,
	maxRetries int,
	writer egress.WriteCloser,
) (egress.WriteCloser, error) {
	return &RetryWriter{
		Writer:        writer,
		retryDuration: retryDuration,
		maxRetries:    maxRetries,
		binding:       urlBinding,
	}, nil
}

// Write will retry writes unitl maxRetries has been reached.
func (r *RetryWriter) Write(e *loggregator_v2.Envelope) error {
	logTemplate := "failed to write to %s, retrying in %s, err: %s"

	var err error

	for i := 0; i < r.maxRetries; i++ {
		err = r.Writer.Write(e)
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
	return r.Writer.Close()
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
