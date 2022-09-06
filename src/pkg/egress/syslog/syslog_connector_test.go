package syslog_test

import (
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	"golang.org/x/net/context"

	"code.cloudfoundry.org/go-loggregator/v9/rpc/loggregator_v2"
	metricsHelpers "code.cloudfoundry.org/go-metric-registry/testhelpers"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/syslog"

	. "github.com/onsi/ginkgo"
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
			Drain: "foo://",
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
			Drain: "slow://",
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
			Drain: "bla://some-domain.tld",
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
			Drain: "://syslog/laksjdflk:asdfdsaf:2232",
		}

		_, err := connector.Connect(ctx, binding)
		Expect(err).To(HaveOccurred())
	})

	It("writes a LGR error for inproperly formatted drains", func() {
		logClient := newSpyLogClient()
		connector := syslog.NewSyslogConnector(
			true,
			spyWaitGroup,
			writerFactory,
			sm,
			syslog.WithLogClient(logClient, "3"),
		)

		binding := syslog.Binding{
			AppId: "some-app-id",
			Drain: "://syslog/laksjdflk:asdfdsaf:2232",
		}

		_, _ = connector.Connect(ctx, binding)

		Expect(logClient.message()).To(ContainElement("Invalid syslog drain URL: parse failure"))
		Expect(logClient.appID()).To(ContainElement("some-app-id"))
		Expect(logClient.sourceType()).To(HaveKey("LGR"))
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
				Drain: "dropping://my-drain:8080/path?secret=1234",
				AppId: "test-source-id",
			}

			writer, err := connector.Connect(ctx, binding)
			Expect(err).ToNot(HaveOccurred())

			//nolint:errcheck
			go func(w egress.Writer) {
				for {
					w.Write(&loggregator_v2.Envelope{
						SourceId: "test-source-id",
						Message: &loggregator_v2.Envelope_Log{
							Log: &loggregator_v2.Log{},
						},
					})
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
			logClient := newSpyLogClient()
			connector := syslog.NewSyslogConnector(
				true,
				spyWaitGroup,
				writerFactory,
				sm,
				syslog.WithLogClient(logClient, "3"),
			)

			binding := syslog.Binding{AppId: "app-id", Drain: "dropping://"}
			writer, err := connector.Connect(ctx, binding)
			Expect(err).ToNot(HaveOccurred())

			go func(w egress.Writer) {
				for {
					err := w.Write(&loggregator_v2.Envelope{
						SourceId: "test-source-id",
						Message: &loggregator_v2.Envelope_Log{
							Log: &loggregator_v2.Log{},
						},
					})
					Expect(err).ToNot(HaveOccurred())
				}
			}(writer)

			Eventually(logClient.message).Should(ContainElement(MatchRegexp("\\d messages lost in user provided syslog drain")))
			Eventually(logClient.appID).Should(ContainElement("app-id"))

			Eventually(logClient.sourceType).Should(HaveLen(2))
			Eventually(logClient.sourceType).Should(HaveKey("LGR"))
			Eventually(logClient.sourceType).Should(HaveKey("SYS"))

			Eventually(logClient.sourceInstance).Should(HaveLen(2))
			Eventually(logClient.sourceInstance).Should(HaveKey(""))
			Eventually(logClient.sourceInstance).Should(HaveKey("3"))
		})

		It("doesn't emit LGR and SYS log to the log client about aggregate drains drops", func() {
			logClient := newSpyLogClient()
			connector := syslog.NewSyslogConnector(
				true,
				spyWaitGroup,
				writerFactory,
				sm,
				syslog.WithLogClient(logClient, "3"),
			)

			binding := syslog.Binding{Drain: "dropping://"}
			writer, err := connector.Connect(ctx, binding)
			Expect(err).ToNot(HaveOccurred())

			go func(w egress.Writer) {
				for {
					err := w.Write(&loggregator_v2.Envelope{
						SourceId: "test-source-id",
						Message: &loggregator_v2.Envelope_Log{
							Log: &loggregator_v2.Log{},
						},
					})
					Expect(err).ToNot(HaveOccurred())
				}
			}(writer)

			Consistently(logClient.message).ShouldNot(ContainElement(MatchRegexp("\\d messages lost in user provided syslog drain")))
		})

		It("does not panic on unknown dropped metrics", func() {
			binding := syslog.Binding{Drain: "dropping://"}

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
					err = writer.Write(&loggregator_v2.Envelope{
						SourceId: "test-source-id",
					})
					Expect(err).ToNot(HaveOccurred())
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
