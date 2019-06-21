package binding_test

import (
	"context"
	"errors"
	"log"
	"strings"
	"sync/atomic"
	"time"

	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	"code.cloudfoundry.org/loggregator-agent/internal/testhelper"
	"code.cloudfoundry.org/loggregator-agent/pkg/binding"
	"code.cloudfoundry.org/loggregator-agent/pkg/egress"
	"code.cloudfoundry.org/loggregator-agent/pkg/egress/syslog"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Manager", func() {
	var (
		stubBindingFetcher *stubBindingFetcher
		spyMetricClient    *testhelper.SpyMetricClient
		spyConnector       *spyConnector

		binding1 = syslog.Binding{AppId: "app-1", Hostname: "host-1", Drain: "syslog://drain.url.com"}
		binding2 = syslog.Binding{AppId: "app-2", Hostname: "host-2", Drain: "syslog://drain.url.com"}
		binding3 = syslog.Binding{AppId: "app-3", Hostname: "host-3", Drain: "syslog://drain.url.com"}
	)

	BeforeEach(func() {
		stubBindingFetcher = newStubBindingFetcher()
		spyMetricClient = testhelper.NewMetricClient()
		spyConnector = newSpyConnector()
	})

	Describe("GetDrains()", func() {
		It("returns drains for a sourceID", func() {
			stubBindingFetcher.bindings <- []syslog.Binding{binding1, binding2, binding3}

			m := binding.NewManager(
				stubBindingFetcher,
				spyConnector,
				nil,
				spyMetricClient, 10*time.Second,
				10*time.Minute,
				log.New(GinkgoWriter, "", 0),
			)
			go m.Run()

			var appDrains []egress.Writer
			Eventually(func() int {
				appDrains = m.GetDrains("app-1")
				return len(appDrains)
			}).Should(Equal(1))

			e := &loggregator_v2.Envelope{
				Timestamp: time.Now().UnixNano(),
				SourceId:  "app-1",
				Message: &loggregator_v2.Envelope_Log{
					Log: &loggregator_v2.Log{
						Payload: []byte("hello"),
					},
				},
			}

			appDrains[0].Write(e)
			Eventually(appDrains[0].(*spyDrain).envelopes).Should(Receive(Equal(e)))
		})

		It("returns universal syslog drains for all sourceIDs", func() {
			stubBindingFetcher.bindings <- []syslog.Binding{binding1}

			m := binding.NewManager(
				stubBindingFetcher,
				spyConnector,
				[]string{"syslog://universal-drain.url.com"},
				spyMetricClient, 10*time.Second,
				10*time.Minute,
				log.New(GinkgoWriter, "", 0),
			)
			go m.Run()

			var appDrains []egress.Writer
			Eventually(func() int {
				appDrains = m.GetDrains("app-1")
				return len(appDrains)
			}).Should(Equal(2))

			e := &loggregator_v2.Envelope{
				Timestamp: time.Now().UnixNano(),
				SourceId:  "app-1",
				Message: &loggregator_v2.Envelope_Log{
					Log: &loggregator_v2.Log{
						Payload: []byte("hello"),
					},
				},
			}

			appDrains[0].Write(e)
			Eventually(appDrains[0].(*spyDrain).envelopes).Should(Receive(Equal(e)))

			appDrains[1].Write(e)
			Eventually(appDrains[1].(*spyDrain).envelopes).Should(Receive(Equal(e)))
		})

		It("creates connections when asked for them", func() {
			stubBindingFetcher.bindings <- []syslog.Binding{
				binding1,
				binding3,
			}
			stubBindingFetcher.bindings <- []syslog.Binding{
				binding1,
				binding2,
				binding3,
			}

			go func(bindings chan []syslog.Binding) {
				for {
					bindings <- []syslog.Binding{
						binding2,
						binding3,
					}
				}
			}(stubBindingFetcher.bindings)

			m := binding.NewManager(
				stubBindingFetcher,
				spyConnector,
				nil,
				spyMetricClient, 100*time.Millisecond,
				10*time.Minute,
				log.New(GinkgoWriter, "", 0),
			)
			go m.Run()

			Eventually(func() []egress.Writer {
				return m.GetDrains("app-1")
			}).Should(HaveLen(1))
			Expect(spyConnector.ConnectionCount()).To(BeNumerically("==", 1))
			Expect(spyMetricClient.GetMetric("active_drains", map[string]string{"unit": "count"}).Value()).To(Equal(1.0))

			Eventually(func() []egress.Writer {
				return m.GetDrains("app-2")
			}).Should(HaveLen(1))
			Expect(spyConnector.ConnectionCount()).To(BeNumerically("==", 2))
			Expect(spyMetricClient.GetMetric("active_drains", map[string]string{"unit": "count"}).Value()).To(Equal(2.0))

			Eventually(func() []egress.Writer {
				return m.GetDrains("app-3")
			}).Should(HaveLen(1))
			Expect(spyConnector.ConnectionCount()).To(BeNumerically("==", 3))
			Expect(spyMetricClient.GetMetric("active_drains", map[string]string{"unit": "count"}).Value()).To(Equal(3.0))
		})
	})

	It("polls for updates from the binding fetcher", func() {
		stubBindingFetcher.bindings <- []syslog.Binding{
			binding1,
			binding3,
		}

		m := binding.NewManager(
			stubBindingFetcher,
			spyConnector,
			nil,
			spyMetricClient, 100*time.Millisecond,
			10*time.Minute,
			log.New(GinkgoWriter, "", 0),
		)
		go m.Run()

		Eventually(func() float64 {
			return spyMetricClient.GetMetric("drains", map[string]string{"unit": "count"}).Value()
		}).Should(BeNumerically("==", 2))

		stubBindingFetcher.bindings <- []syslog.Binding{
			binding1,
			binding2,
			binding3,
		}

		Eventually(func() float64 {
			return spyMetricClient.GetMetric("drains", map[string]string{"unit": "count"}).Value()
		}).Should(BeNumerically("==", 3))

		go func(bindings chan []syslog.Binding) {
			for {
				bindings <- []syslog.Binding{
					binding2,
					binding3,
				}
			}
		}(stubBindingFetcher.bindings)

		Eventually(func() []egress.Writer {
			return m.GetDrains("app-1")
		}).Should(HaveLen(1))
		Eventually(func() []egress.Writer {
			return m.GetDrains("app-2")
		}).Should(HaveLen(1))
		Eventually(func() []egress.Writer {
			return m.GetDrains("app-3")
		}).Should(HaveLen(1))
		Expect(spyConnector.ConnectionCount()).Should(BeNumerically("==", 3))

		// Also remove old drains when updating
		Eventually(func() []egress.Writer {
			return m.GetDrains("app-1")
		}).Should(HaveLen(0))

		closedBdg := binding1
		closedCtx := spyConnector.bindingContextMap[closedBdg]
		Expect(closedCtx.Err()).To(Equal(errors.New("context canceled")))
	})

	It("reports the number of bindings that come from the fetcher", func() {
		stubBindingFetcher.bindings <- []syslog.Binding{
			binding1,
			binding2,
			binding3,
		}

		m := binding.NewManager(
			stubBindingFetcher,
			spyConnector,
			nil,
			spyMetricClient, 100*time.Millisecond,
			10*time.Minute,
			log.New(GinkgoWriter, "", 0),
		)
		go m.Run()

		Eventually(func() float64 {
			return spyMetricClient.GetMetric("drains", map[string]string{"unit": "count"}).Value()
		}).Should(BeNumerically("==", 3))
	})

	It("reports the number of universal drains", func() {
		binding.NewManager(
			stubBindingFetcher,
			spyConnector,
			[]string{
				"syslog://universal-drain1.url.com",
				"syslog://universal-drain2.url.com",
			},
			spyMetricClient, 100*time.Millisecond,
			10*time.Minute,
			log.New(GinkgoWriter, "", 0),
		)

		Expect(spyMetricClient.GetMetric("non_app_drains", map[string]string{"unit": "count"}).Value()).To(Equal(float64(2)))
	})

	It("includes universal drains in active drain count", func() {
		stubBindingFetcher.bindings <- []syslog.Binding{
			binding1,
			binding2,
			binding3,
		}

		m := binding.NewManager(
			stubBindingFetcher,
			spyConnector,
			[]string{
				"syslog://universal-drain1.url.com",
			},
			spyMetricClient, 100*time.Millisecond,
			10*time.Minute,
			log.New(GinkgoWriter, "", 0),
		)
		go m.Run()

		Eventually(func() []egress.Writer {
			return m.GetDrains("app-1")
		}).Should(HaveLen(2))
		Expect(spyConnector.ConnectionCount()).To(BeNumerically("==", 2))

		Eventually(func() []egress.Writer {
			return m.GetDrains("app-2")
		}).Should(HaveLen(2))
		Expect(spyConnector.ConnectionCount()).To(BeNumerically("==", 3))

		Expect(spyMetricClient.GetMetric("active_drains", map[string]string{"unit": "count"}).Value()).To(Equal(3.0))
	})

	It("removes deleted drains", func() {
		stubBindingFetcher.bindings <- []syslog.Binding{
			binding1,
			binding2,
			binding3,
		}

		m := binding.NewManager(
			stubBindingFetcher,
			spyConnector,
			nil,
			spyMetricClient, 100*time.Millisecond,
			10*time.Minute,
			log.New(GinkgoWriter, "", 0),
		)
		go m.Run()

		Eventually(func() []egress.Writer {
			return m.GetDrains("app-1")
		}).Should(HaveLen(1))
		Expect(spyMetricClient.GetMetric("active_drains", map[string]string{"unit": "count"}).Value()).To(Equal(1.0))

		go func(bindings chan []syslog.Binding) {
			for {
				bindings <- []syslog.Binding{
					binding2,
					binding3,
				}
			}
		}(stubBindingFetcher.bindings)

		Eventually(func() []egress.Writer {
			return m.GetDrains("app-1")
		}).Should(HaveLen(0))
		Expect(spyMetricClient.GetMetric("active_drains", map[string]string{"unit": "count"}).Value()).To(Equal(0.0))
	})

	It("removes drain holders for inactive drains", func() {
		stubBindingFetcher.bindings <- []syslog.Binding{
			binding1,
			{"app-2", "host-1", "syslog://drain.url.com"},
		}

		m := binding.NewManager(
			stubBindingFetcher,
			spyConnector,
			nil,
			spyMetricClient, 100*time.Millisecond,
			100*time.Millisecond,
			log.New(GinkgoWriter, "", 0),
		)
		go m.Run()

		Eventually(func() []egress.Writer {
			return m.GetDrains("app-1")
		}).Should(HaveLen(1))

		go func() {
			for {
				Eventually(func() []egress.Writer {
					return m.GetDrains("app-2")
				}).Should(HaveLen(1))
			}
		}()

		Eventually(func() float64 {
			return spyMetricClient.GetMetric("active_drains", map[string]string{"unit": "count"}).Value()
		}).Should(Equal(2.0))

		// app-1 should eventually expire and be cleaned up.
		Eventually(func() float64 {
			return spyMetricClient.GetMetric("active_drains", map[string]string{"unit": "count"}).Value()
		}).Should(Equal(1.0))

		// The active drain count metric should only be decremented once.
		Consistently(func() float64 {
			return spyMetricClient.GetMetric("active_drains", map[string]string{"unit": "count"}).Value()
		}).Should(Equal(1.0))

		// It re-activates on another get drains.
		Eventually(func() []egress.Writer {
			return m.GetDrains("app-1")
		}).Should(HaveLen(1))

		Eventually(func() float64 {
			return spyMetricClient.GetMetric("active_drains", map[string]string{"unit": "count"}).Value()
		}).Should(Equal(2.0))
	})

	It("maintains current state on error", func() {
		stubBindingFetcher.bindings <- []syslog.Binding{
			binding1,
		}

		m := binding.NewManager(
			stubBindingFetcher,
			spyConnector,
			nil,
			spyMetricClient, 10*time.Millisecond,
			10*time.Minute,
			log.New(GinkgoWriter, "", 0),
		)
		go m.Run()

		Eventually(func() int {
			return len(m.GetDrains("app-1"))
		}).Should(Equal(1))

		stubBindingFetcher.errors <- errors.New("boom")

		Consistently(func() int {
			return len(m.GetDrains("app-1"))
		}).Should(Equal(1))
	})

	It("should not return a drain for binding to an invalid address", func() {
		stubBindingFetcher.bindings <- []syslog.Binding{
			{"app-1", "host-1", "syslog-v3-v3://drain.url.com"},
		}

		m := binding.NewManager(
			stubBindingFetcher,
			spyConnector,
			nil,
			spyMetricClient, 10*time.Millisecond,
			10*time.Minute,
			log.New(GinkgoWriter, "", 0),
		)
		go m.Run()

		Consistently(func() []egress.Writer {
			return m.GetDrains("app-1")
		}).Should(HaveLen(0))
	})
})

type spyDrain struct {
	envelopes chan *loggregator_v2.Envelope
}

func newSpyDrain() *spyDrain {
	return &spyDrain{
		envelopes: make(chan *loggregator_v2.Envelope, 100),
	}
}

func (s *spyDrain) Write(e *loggregator_v2.Envelope) error {
	s.envelopes <- e
	return nil
}

type spyConnector struct {
	connectionCount   int64
	bindingContextMap map[syslog.Binding]context.Context
}

func newSpyConnector() *spyConnector {
	return &spyConnector{
		bindingContextMap: make(map[syslog.Binding]context.Context),
	}
}

func (c *spyConnector) ConnectionCount() int64 {
	return atomic.LoadInt64(&c.connectionCount)
}

func (c *spyConnector) Connect(ctx context.Context, b syslog.Binding) (egress.Writer, error) {
	if strings.HasPrefix(b.Drain, "syslog://") {
		c.bindingContextMap[b] = ctx
		atomic.AddInt64(&c.connectionCount, 1)
		return newSpyDrain(), nil
	}

	return nil, errors.New("invalid hostname")
}

type stubBindingFetcher struct {
	bindings chan []syslog.Binding
	errors   chan error
}

func newStubBindingFetcher() *stubBindingFetcher {
	return &stubBindingFetcher{
		bindings: make(chan []syslog.Binding, 100),
		errors:   make(chan error, 100),
	}
}

func (s *stubBindingFetcher) FetchBindings() ([]syslog.Binding, error) {
	select {
	case b := <-s.bindings:
		return b, nil
	case err := <-s.errors:
		return nil, err
	}
}

func (s *stubBindingFetcher) DrainLimit() int {
	return 100
}
