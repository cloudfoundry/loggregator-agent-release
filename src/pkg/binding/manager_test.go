package binding_test

import (
	"context"
	"errors"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"code.cloudfoundry.org/go-loggregator/v8/rpc/loggregator_v2"
	metricsHelpers "code.cloudfoundry.org/go-metric-registry/testhelpers"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/binding"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/syslog"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Manager", func() {
	var (
		stubAppBindingFetcher       *stubBindingFetcher
		stubAggregateBindingFetcher *stubBindingFetcher
		spyMetricClient             *metricsHelpers.SpyMetricsRegistry
		spyConnector                *spyConnector

		binding1 = syslog.Binding{AppId: "app-1", Hostname: "host-1", Drain: "syslog://drain.url.com"}
		binding2 = syslog.Binding{AppId: "app-2", Hostname: "host-2", Drain: "syslog://drain.url.com"}
		binding3 = syslog.Binding{AppId: "app-3", Hostname: "host-3", Drain: "syslog://drain.url.com"}

		aggregateBinding1 = syslog.Binding{AppId: "", Hostname: "host-1", Drain: "syslog://aggregate1.url.com"}
		aggregateBinding2 = syslog.Binding{AppId: "", Hostname: "host-1", Drain: "syslog://aggregate2.url.com"}
	)

	BeforeEach(func() {
		stubAppBindingFetcher = newStubBindingFetcher()
		stubAggregateBindingFetcher = newStubBindingFetcher()
		spyMetricClient = metricsHelpers.NewMetricsRegistry()
		spyConnector = newSpyConnector()
	})

	Describe("GetDrains()", func() {
		It("returns drains for a sourceID", func() {
			stubAppBindingFetcher.bindings <- []syslog.Binding{binding1, binding2, binding3}

			m := binding.NewManager(
				stubAppBindingFetcher,
				stubAggregateBindingFetcher,
				spyConnector,
				spyMetricClient, 10*time.Second,
				10*time.Minute,
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

		It("returns aggregate syslog drains for all sourceIDs", func() {
			stubAppBindingFetcher.bindings <- []syslog.Binding{binding1}
			stubAggregateBindingFetcher.bindings <- []syslog.Binding{aggregateBinding1}

			m := binding.NewManager(
				stubAppBindingFetcher,
				stubAggregateBindingFetcher,
				spyConnector,
				spyMetricClient, 10*time.Second,
				10*time.Minute,
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
			stubAppBindingFetcher.bindings <- []syslog.Binding{
				binding1,
				binding3,
			}
			stubAppBindingFetcher.bindings <- []syslog.Binding{
				binding1,
				binding2,
				binding3,
			}
			stubAggregateBindingFetcher.bindings <- []syslog.Binding{}

			go func(bindings chan []syslog.Binding) {
				for {
					bindings <- []syslog.Binding{
						binding2,
						binding3,
					}
				}
			}(stubAppBindingFetcher.bindings)

			m := binding.NewManager(
				stubAppBindingFetcher,
				stubAggregateBindingFetcher,
				spyConnector,
				spyMetricClient,
				100*time.Millisecond,
				10*time.Minute,
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
		stubAppBindingFetcher.bindings <- []syslog.Binding{
			binding1,
			binding3,
		}
		stubAggregateBindingFetcher.bindings <- []syslog.Binding{}

		m := binding.NewManager(
			stubAppBindingFetcher,
			stubAggregateBindingFetcher,
			spyConnector,
			spyMetricClient,
			100*time.Millisecond,
			10*time.Minute,
			10*time.Minute,
			log.New(GinkgoWriter, "", 0),
		)
		go m.Run()

		Eventually(func() float64 {
			return spyMetricClient.GetMetric("drains", map[string]string{"unit": "count"}).Value()
		}).Should(BeNumerically("==", 2))

		stubAppBindingFetcher.bindings <- []syslog.Binding{
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
		}(stubAppBindingFetcher.bindings)

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
		stubAppBindingFetcher.bindings <- []syslog.Binding{
			binding1,
			binding2,
			binding3,
		}

		m := binding.NewManager(
			stubAppBindingFetcher,
			stubAggregateBindingFetcher,
			spyConnector,
			spyMetricClient,
			100*time.Millisecond,
			10*time.Minute,
			10*time.Minute,
			log.New(GinkgoWriter, "", 0),
		)
		go m.Run()

		Eventually(func() float64 {
			return spyMetricClient.GetMetric("drains", map[string]string{"unit": "count"}).Value()
		}).Should(BeNumerically("==", 3))
	})

	It("reports the number of aggregate drains", func() {
		stubAppBindingFetcher.bindings <- []syslog.Binding{}
		stubAggregateBindingFetcher.bindings <- []syslog.Binding{aggregateBinding1, aggregateBinding2}

		m := binding.NewManager(
			stubAppBindingFetcher,
			stubAggregateBindingFetcher,
			spyConnector,
			spyMetricClient,
			100*time.Millisecond,
			10*time.Minute,
			10*time.Minute,
			log.New(GinkgoWriter, "", 0),
		)
		go m.Run()

		Eventually(func() float64 {
			return spyMetricClient.GetMetric("aggregate_drains", map[string]string{"unit": "count"}).Value()

		}).Should(Equal(float64(2)))
	})

	It("includes aggregate drains in active drain count", func() {
		stubAppBindingFetcher.bindings <- []syslog.Binding{
			binding1,
			binding2,
			binding3,
		}
		stubAggregateBindingFetcher.bindings <- []syslog.Binding{aggregateBinding1}

		m := binding.NewManager(
			stubAppBindingFetcher,
			stubAggregateBindingFetcher,
			spyConnector,
			spyMetricClient,
			time.Hour,
			10*time.Minute,
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

	It("re-connects the aggregate drains after configured interval", func() {
		stubAppBindingFetcher.bindings <- []syslog.Binding{}
		stubAppBindingFetcher.bindings <- []syslog.Binding{}
		stubAggregateBindingFetcher.bindings <- []syslog.Binding{aggregateBinding1}
		stubAggregateBindingFetcher.bindings <- []syslog.Binding{aggregateBinding1}

		m := binding.NewManager(
			stubAppBindingFetcher,
			stubAggregateBindingFetcher,
			spyConnector,
			spyMetricClient,
			10*time.Minute,
			10*time.Minute,
			100*time.Millisecond,
			log.New(GinkgoWriter, "", 0),
		)
		go m.Run()

		var appDrains []egress.Writer
		Eventually(func() int {
			appDrains = m.GetDrains("app-1")
			return len(appDrains)
		}).Should(Equal(1))

		Eventually(func() egress.Writer {
			newAppDrains := m.GetDrains("app-1")
			Expect(newAppDrains).To(HaveLen(1))
			return newAppDrains[0]
		}).ShouldNot(Equal(appDrains[0]))
	})

	It("removes deleted drains", func() {
		stubAppBindingFetcher.bindings <- []syslog.Binding{
			binding1,
			binding2,
			binding3,
		}
		stubAggregateBindingFetcher.bindings <- []syslog.Binding{}

		m := binding.NewManager(
			stubAppBindingFetcher,
			stubAggregateBindingFetcher,
			spyConnector,
			spyMetricClient, 100*time.Millisecond,
			10*time.Minute,
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
		}(stubAppBindingFetcher.bindings)

		Eventually(func() []egress.Writer {
			return m.GetDrains("app-1")
		}).Should(HaveLen(0))
		Expect(spyMetricClient.GetMetric("active_drains", map[string]string{"unit": "count"}).Value()).To(Equal(0.0))
	})

	It("removes drain holders for inactive drains", func() {
		stubAppBindingFetcher.bindings <- []syslog.Binding{
			binding1,
			{AppId: "app-2", Hostname: "host-1", Drain: "syslog://drain.url.com"},
		}

		m := binding.NewManager(
			stubAppBindingFetcher,
			stubAggregateBindingFetcher,
			spyConnector,
			spyMetricClient,
			100*time.Millisecond,
			100*time.Millisecond,
			10*time.Minute,
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

	It("bad drains don't report on active drains", func() {
		stubAppBindingFetcher.bindings <- []syslog.Binding{
			{AppId: "app-1", Hostname: "host-1", Drain: "bad://drain1.url.com"},
			{AppId: "app-1", Hostname: "host-2", Drain: "bad://drain2.url.com"},
		}

		m := binding.NewManager(
			stubAppBindingFetcher,
			stubAggregateBindingFetcher,
			spyConnector,
			spyMetricClient,
			10*time.Millisecond,
			10*time.Millisecond,
			10*time.Minute,
			log.New(GinkgoWriter, "", 0),
		)
		go m.Run()

		Eventually(func() []egress.Writer {
			return m.GetDrains("app-1")
		}).Should(HaveLen(0))

		Consistently(func() float64 {
			return spyMetricClient.GetMetric("active_drains", map[string]string{"unit": "count"}).Value()
		}).Should(Equal(0.0))
	})

	It("maintains current state on error", func() {
		stubAppBindingFetcher.bindings <- []syslog.Binding{
			binding1,
		}

		m := binding.NewManager(
			stubAppBindingFetcher,
			stubAggregateBindingFetcher,
			spyConnector,
			spyMetricClient, 10*time.Millisecond,
			10*time.Minute,
			10*time.Minute,
			log.New(GinkgoWriter, "", 0),
		)
		go m.Run()

		Eventually(func() int {
			return len(m.GetDrains("app-1"))
		}).Should(Equal(1))

		stubAppBindingFetcher.errors <- errors.New("boom")

		Consistently(func() int {
			return len(m.GetDrains("app-1"))
		}).Should(Equal(1))
	})

	It("should not return a drain for binding to an invalid address", func() {
		stubAppBindingFetcher.bindings <- []syslog.Binding{
			{AppId: "app-1", Hostname: "host-1", Drain: "syslog-v3-v3://drain.url.com"},
		}

		m := binding.NewManager(
			stubAppBindingFetcher,
			stubAggregateBindingFetcher,
			spyConnector,
			spyMetricClient, 10*time.Millisecond,
			10*time.Minute,
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
	mu                   sync.Mutex
	connectionCount      int64
	bindingConnectedList []syslog.Binding
	bindingContextMap    map[syslog.Binding]context.Context
}

func newSpyConnector() *spyConnector {
	return &spyConnector{
		bindingContextMap:    make(map[syslog.Binding]context.Context),
		bindingConnectedList: make([]syslog.Binding, 0),
	}
}

func (c *spyConnector) BindingsConnected() []syslog.Binding {
	c.mu.Lock()
	defer c.mu.Unlock()

	var connectedListToReturn []syslog.Binding
	connectedListToReturn = append(connectedListToReturn, c.bindingConnectedList...)
	return connectedListToReturn
}

func (c *spyConnector) ConnectionCount() int64 {
	return atomic.LoadInt64(&c.connectionCount)
}

func (c *spyConnector) Connect(ctx context.Context, b syslog.Binding) (egress.Writer, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if strings.HasPrefix(b.Drain, "syslog://") {
		c.bindingContextMap[b] = ctx
		atomic.AddInt64(&c.connectionCount, 1)
		c.bindingConnectedList = append(c.bindingConnectedList, b)
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
