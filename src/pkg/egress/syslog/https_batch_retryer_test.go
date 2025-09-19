package syslog_test

import (
	"errors"
	"net/url"
	"sync"
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
		coordinator   *syslog.RetryCoordinator
		retryInterval time.Duration
		waitTime      time.Duration
	)

	BeforeEach(func() {
		retryAttempts = 0
		retryInterval = 10 * time.Millisecond
		waitTime = retryInterval * 5
		syslog.WithParallelRetries(2)
		coordinator = syslog.GetGlobalRetryCoordinator()
		binding = &syslog.URLBinding{
			URL: &url.URL{
				Host: "test-host",
			},
			Context: context.Background(),
		}
		retryer = syslog.NewRetryer(
			binding,
			func(attempt int) time.Duration {
				return retryInterval
			}, 3)
	})

	It("retries the specified number of times on failure", func() {
		retryer.Retry([]byte("test-batch"), 10, func(batch []byte, msgCount float64) error {
			retryAttempts++
			return errors.New("test error")
		})

		Expect(retryAttempts).To(Equal(3)) // Retries up to maxRetries
	})

	It("stops retrying when the function succeeds", func() {
		retryer.Retry([]byte("test-batch"), 10, func(batch []byte, msgCount float64) error {
			retryAttempts++
			if retryAttempts == 2 {
				return nil // Succeed on the second attempt
			}
			return errors.New("test error")
		})

		Eventually(retryAttempts, waitTime).Should(Equal(2))
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
		Eventually(retryAttempts, waitTime).Should(Equal(1))
	})

	It("returns the last error after exhausting retries", func() {
		retryer.Retry([]byte("test-batch"), 10, func(batch []byte, msgCount float64) error {
			retryAttempts++
			return errors.New("test error")
		})

		Eventually(retryAttempts, waitTime).Should(Equal(3))
	})

	It("respects the global parallel retry limit (locking behaviour)", func() {
		syslog.WithParallelRetries(2)

		var (
			started sync.WaitGroup
			blocked sync.WaitGroup
			done    sync.WaitGroup
		)

		started.Add(2)
		blocked.Add(1)
		done.Add(2)

		// First two retriers should acquire slots
		go func() {
			started.Done()
			coordinator.Acquire("test-host", "test-app")
			blocked.Wait()
			coordinator.Release()
			done.Done()
		}()
		go func() {
			started.Done()
			coordinator.Acquire("test-host", "test-app")
			blocked.Wait()
			coordinator.Acquire("test-host", "test-app")
			done.Done()
		}()

		started.Wait()

		// Third retrier should block until a slot is released
		acquired := make(chan int)
		go func() {
			coordinator.Acquire("test-host", "test-app")
			close(acquired)
			coordinator.Release()
		}()

		Consistently(acquired, 
			200time.Millisecond,20time.Millisecond).ShouldNot(BeClosed())
		

		// Unblock the first two
		blocked.Done()
		done.Wait()

		Eventually(acquired, 100*time.Millisecond).Should(BeClosed())
	})
})
