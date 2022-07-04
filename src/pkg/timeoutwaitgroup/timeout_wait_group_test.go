package timeoutwaitgroup_test

import (
	"time"

	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/timeoutwaitgroup"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("TimeoutWaitGroup", func() {
	It("returns immediately if there is nothing to wait on", func() {
		done := make(chan interface{})

		go func() {
			defer GinkgoRecover()
			defer close(done)

			waiter := timeoutwaitgroup.New(time.Minute)

			startTime := time.Now()
			waiter.Wait()

			Expect(time.Since(startTime)).To(BeNumerically("<", 100*time.Millisecond))
		}()
		Eventually(done, 1).Should(BeClosed())
	})

	It("blocks for up to the timeout if there is something to wait on", func() {
		done := make(chan interface{})

		go func() {
			defer GinkgoRecover()
			defer close(done)

			waiter := timeoutwaitgroup.New(50 * time.Millisecond)

			waiter.Add(1)

			startTime := time.Now()
			waiter.Wait()
			Expect(time.Since(startTime)).To(BeNumerically(">", 50*time.Millisecond))
		}()
		Eventually(done, 1).Should(BeClosed())
	})

	It("returns before the timeout if everything finishes", func() {
		done := make(chan interface{})

		go func() {
			defer GinkgoRecover()
			defer close(done)

			waiter := timeoutwaitgroup.New(time.Minute)
			waiter.Add(1)

			startTime := time.Now()
			go func() {
				time.Sleep(20 * time.Millisecond)
				waiter.Done()
			}()
			waiter.Wait()
			Expect(time.Since(startTime)).To(BeNumerically("<", 100*time.Millisecond))
		}()
		Eventually(done, 1).Should(BeClosed())
	})
})
