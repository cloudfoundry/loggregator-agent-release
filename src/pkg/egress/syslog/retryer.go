package syslog

import (
	"log"
	"time"

	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress"
)

// RetryWriter wraps a WriteCloser and will retry writes if the first fails.
type Retryer struct {
	retryDuration RetryDuration
	maxRetries    int
	binding       *URLBinding
}

func NewBackoffRetryer(
	urlBinding *URLBinding,
	retryDuration RetryDuration,
	maxRetries int,
) *Retryer {
	return &Retryer{
		retryDuration: retryDuration,
		maxRetries:    maxRetries,
		binding:       urlBinding,
	}
}

// Write will retry writes unitl maxRetries has been reached.
func (r *Retryer) Retry(message []byte, fn func(msg []byte) error) error {
	logTemplate := "failed to write to %s, retrying in %s, err: %s"

	var err error

	for i := 0; i < r.maxRetries; i++ {
		err = fn(message)
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
