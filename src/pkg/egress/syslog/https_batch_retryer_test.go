package syslog_test

import (
	"errors"
	"net/url"
	"time"

	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/syslog"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/net/context"
)

var _ = Describe("Retryer", func() {
	var (
		retryer       *syslog.Retryer
		retryAttempts int
		binding       *syslog.URLBinding
	)

	BeforeEach(func() {
		retryAttempts = 0
		binding = &syslog.URLBinding{
			URL: &url.URL{
				Host: "test-host",
			},
			Context: context.Background(),
		}
		retryer = syslog.NewRetryer(
			binding,
			func(attempt int) time.Duration {
				return 10 * time.Millisecond
			}, 3)
	})

	It("retries the specified number of times on failure", func() {
		retryer.Retry([]byte("test-batch"), 10, func(batch []byte, msgCount float64) error {
			retryAttempts++
			return errors.New("test error")
		})

		Expect(retryAttempts).To(Equal(4)) // Retries up to maxRetries
	})

	It("stops retrying when the function succeeds", func() {
		retryer.Retry([]byte("test-batch"), 10, func(batch []byte, msgCount float64) error {
			retryAttempts++
			if retryAttempts == 2 {
				return nil // Succeed on the second attempt
			}
			return errors.New("test error")
		})

		Expect(retryAttempts).To(Equal(2)) // Stops after success
	})

	It("stops retrying when the context is canceled", func() {
		ctx, cancel := context.WithCancel(context.Background())
		binding.Context = ctx
		retryer = syslog.NewRetryer(
			binding,
			func(attempt int) time.Duration {
				return 10 * time.Millisecond
			}, 3)

		cancel() // Cancel the context
		retryer.Retry([]byte("test-batch"), 10, func(batch []byte, msgCount float64) error {
			retryAttempts++
			return errors.New("test error")
		})
		Expect(retryAttempts).To(Equal(1)) // Only one attempt due to context cancellation
	})

	It("returns the last error after exhausting retries", func() {
		retryer.Retry([]byte("test-batch"), 10, func(batch []byte, msgCount float64) error {
			retryAttempts++
			return errors.New("test error")
		})

		Expect(retryAttempts).To(Equal(4)) // Retries up to maxRetries
	})
})
