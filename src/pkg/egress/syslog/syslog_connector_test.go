package syslog_test

import (
	"code.cloudfoundry.org/loggregator-agent-release/src/internal/testhelper"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/applog"

	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	"golang.org/x/net/context"

	"code.cloudfoundry.org/go-loggregator/v10/rpc/loggregator_v2"
	metricsHelpers "code.cloudfoundry.org/go-metric-registry/testhelpers"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/syslog"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("SyslogConnector", func() {
	var (
		ctx           context.Context
		spyWaitGroup  *SpyWaitGroup
		writerFactory *stubWriterFactory
		sm            *metricsHelpers.SpyMetricsRegistry
	)

	BeforeEach(func() {
		sm = metricsHelpers.NewMetricsRegistry()
		ctx, _ = context.WithCancel(context.Background())
		spyWaitGroup = &SpyWaitGroup{}
		writerFactory = &stubWriterFactory{}
	})

	It("connects to the passed syslog protocol", func() {
		writerFactory.writer = &SleepWriterCloser{metric: func(uint64) {}}
		connector := syslog.NewSyslogConnector(
			true,
			spyWaitGroup,
			writerFactory,
			sm,
		)

		binding := syslog.Binding{
			Drain: syslog.Drain{
				Url: "foo://",
			},
		}
		_, err := connector.Connect(ctx, binding)
		Expect(err).ToNot(HaveOccurred())
		Expect(writerFactory.called).To(BeTrue())
	})

	It("returns a writer that doesn't block even if the constructor's writer blocks", func() {
		writerFactory.writer = &SleepWriterCloser{
			metric:   func(uint64) {},
			duration: time.Hour,
		}

		connector := syslog.NewSyslogConnector(
			true,
			spyWaitGroup,
			writerFactory,
			sm,
		)

		binding := syslog.Binding{
			Drain: syslog.Drain{
				Url: "slow://",
			},
		}
		writer, err := connector.Connect(ctx, binding)
		Expect(err).ToNot(HaveOccurred())
		err = writer.Write(&loggregator_v2.Envelope{
			SourceId: "test-source-id",
		})
		Expect(err).ToNot(HaveOccurred())
	})

	It("returns an error when the writer factory returns an error", func() {
		writerFactory.err = errors.New("unsupported protocol")
		connector := syslog.NewSyslogConnector(
			true,
			spyWaitGroup,
			writerFactory,
			sm,
		)

		binding := syslog.Binding{
			Drain: syslog.Drain{
				Url: "bla://some-domain.tld",
			},
		}
		_, err := connector.Connect(ctx, binding)
		Expect(err).To(MatchError("unsupported protocol"))
	})

	It("returns an error for an inproperly formatted drain", func() {
		connector := syslog.NewSyslogConnector(
			true,
			spyWaitGroup,
			writerFactory,
			sm,
		)

		binding := syslog.Binding{
			Drain: syslog.Drain{
				Url: "://syslog/laksjdflk:asdfdsaf:2232",
			},
		}

		_, err := connector.Connect(ctx, binding)
		Expect(err).To(HaveOccurred())
	})

	Describe("dropping messages", func() {
		BeforeEach(func() {
			writerFactory.writer = &SleepWriterCloser{
				metric:   func(uint64) {},
				duration: time.Millisecond,
			}
		})

		It("emits a metric on dropped messages", func() {
			connector := syslog.NewSyslogConnector(
				true,
				spyWaitGroup,
				writerFactory,
				sm,
			)

			binding := syslog.Binding{
				Drain: syslog.Drain{
					Url: "dropping://my-drain:8080/path?secret=1234",
				},
				AppId: "test-source-id",
			}

			writer, err := connector.Connect(ctx, binding)
			Expect(err).ToNot(HaveOccurred())

			go func(w egress.Writer) {
				for {
					e := w.Write(&loggregator_v2.Envelope{
						SourceId: "test-source-id",
						Message: &loggregator_v2.Envelope_Log{
							Log: &loggregator_v2.Log{},
						},
					})
					Expect(e).ToNot(HaveOccurred())
				}
			}(writer)

			metric := sm.GetMetric("dropped", map[string]string{"direction": "egress"})
			Expect(metric).ToNot(BeNil())
			Eventually(metric.Value).Should(BeNumerically(">=", 10000))

			Eventually(func() bool {
				return sm.HasMetric("messages_dropped_per_drain", map[string]string{
					"direction":   "egress",
					"drain_scope": "app",
					"drain_url":   "dropping://my-drain:8080/path",
				})
			}).Should(BeTrue(), fmt.Sprintf("%#v", sm.Metrics))

			Eventually(sm.GetMetric("messages_dropped_per_drain", map[string]string{
				"direction":   "egress",
				"drain_scope": "app",
				"drain_url":   "dropping://my-drain:8080/path",
			}).Value).Should(BeNumerically(">=", 10000))
		})

		It("emits a LGR and SYS log to the log client about logs that have been dropped", func() {
			logClient := testhelper.NewSpyLogClient()
			factory := applog.NewAppLogEmitterFactory()
			connector := syslog.NewSyslogConnector(
				true,
				spyWaitGroup,
				writerFactory,
				sm,
				syslog.WithAppLogEmitter(factory.NewAppLogEmitter(logClient, "3")),
			)

			binding := syslog.Binding{AppId: "app-id",
				Drain: syslog.Drain{
					Url: "dropping://",
				},
			}
			writer, err := connector.Connect(ctx, binding)
			Expect(err).ToNot(HaveOccurred())

			go func(w egress.Writer) {
				for {
					e := w.Write(&loggregator_v2.Envelope{
						SourceId: "test-source-id",
						Message: &loggregator_v2.Envelope_Log{
							Log: &loggregator_v2.Log{},
						},
					})
					Expect(e).ToNot(HaveOccurred())
				}
			}(writer)

			Eventually(logClient.Message).Should(ContainElement(MatchRegexp("\\d messages lost for application (.*) in user provided syslog drain with url")))
			Eventually(logClient.AppID).Should(ContainElement("app-id"))

			Eventually(logClient.SourceType).Should(HaveLen(2))
			Eventually(logClient.SourceType).Should(HaveKey("LGR"))
			Eventually(logClient.SourceType).Should(HaveKey("SYS"))

			Eventually(logClient.SourceInstance).Should(HaveLen(2))
			Eventually(logClient.SourceInstance).Should(HaveKey(""))
			Eventually(logClient.SourceInstance).Should(HaveKey("3"))
		})

		It("doesn't emit LGR and SYS log to the log client about aggregate drains drops", func() {
			logClient := testhelper.NewSpyLogClient()
			factory := applog.NewAppLogEmitterFactory()
			connector := syslog.NewSyslogConnector(
				true,
				spyWaitGroup,
				writerFactory,
				sm,
				syslog.WithAppLogEmitter(factory.NewAppLogEmitter(logClient, "3")),
			)

			binding := syslog.Binding{Drain: syslog.Drain{Url: "dropping://"}}
			writer, err := connector.Connect(ctx, binding)
			Expect(err).ToNot(HaveOccurred())

			go func(w egress.Writer) {
				for {
					e := w.Write(&loggregator_v2.Envelope{
						SourceId: "test-source-id",
						Message: &loggregator_v2.Envelope_Log{
							Log: &loggregator_v2.Log{},
						},
					})
					Expect(e).ToNot(HaveOccurred())
				}
			}(writer)

			Consistently(logClient.Message()).ShouldNot(ContainElement(MatchRegexp("\\d messages lost for application (.*) in user provided syslog drain with url")))
		})

		It("does not panic on unknown dropped metrics", func() {
			binding := syslog.Binding{Drain: syslog.Drain{Url: "dropping://"}}

			connector := syslog.NewSyslogConnector(
				true,
				spyWaitGroup,
				writerFactory,
				sm,
			)

			writer, err := connector.Connect(ctx, binding)
			Expect(err).ToNot(HaveOccurred())

			f := func() {
				for i := 0; i < 50000; i++ {
					e := writer.Write(&loggregator_v2.Envelope{
						SourceId: "test-source-id",
					})
					Expect(e).ToNot(HaveOccurred())
				}
			}
			Expect(f).ToNot(Panic())
		})
	})
})

type stubWriterFactory struct {
	called bool
	writer egress.WriteCloser
	err    error
}

func (f *stubWriterFactory) NewWriter(
	urlBinding *syslog.URLBinding,
	emitter applog.AppLogEmitter,
) (egress.WriteCloser, error) {
	f.called = true
	return f.writer, f.err
}

type SleepWriterCloser struct {
	duration time.Duration
	io.Closer
	metric func(uint64)
}

func (c *SleepWriterCloser) Write(*loggregator_v2.Envelope) error {
	c.metric(1)
	time.Sleep(c.duration)
	return nil
}

type SpyWaitGroup struct {
	addInput   int64
	doneCalled int64
}

func (s *SpyWaitGroup) Add(delta int) {
	atomic.AddInt64(&s.addInput, int64(delta))
}

func (s *SpyWaitGroup) Done() {
	atomic.AddInt64(&s.doneCalled, 1)
}

func (s *SpyWaitGroup) AddInput() int64 {
	return atomic.LoadInt64(&s.addInput)
}

func (s *SpyWaitGroup) DoneCalled() int64 {
	return atomic.LoadInt64(&s.doneCalled)
}
