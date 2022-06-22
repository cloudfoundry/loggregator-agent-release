package syslog_test

import (
	"time"

	"code.cloudfoundry.org/go-loggregator/v9/rpc/loggregator_v2"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/syslog"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
)

var _ = Describe("Filtering Drain Writer", func() {
	It("filters all envelope types except log", func() {
		binding := syslog.Binding{AppId: "app-1", Hostname: "host-1", Drain: "syslog://drain.url.com", Type: syslog.BINDING_TYPE_LOG}
		fakeWriter := &fakeWriter{}

		logEnv := &loggregator_v2.Envelope{
			Timestamp: time.Now().UnixNano(),
			SourceId:  "app-1",
			Message: &loggregator_v2.Envelope_Log{
				Log: &loggregator_v2.Log{
					Payload: []byte("hello"),
				},
			},
		}

		metricEnv := &loggregator_v2.Envelope{
			Timestamp: time.Now().UnixNano(),
			SourceId:  "app-1",
			Message: &loggregator_v2.Envelope_Counter{
				Counter: &loggregator_v2.Counter{
					Name:  "some-counter",
					Delta: 1,
					Total: 10,
				},
			},
		}

		drain, err := syslog.NewFilteringDrainWriter(binding, fakeWriter)
		Expect(err).ToNot(HaveOccurred())

		err = drain.Write(logEnv)
		Expect(err).To(BeNil())
		err = drain.Write(metricEnv)
		Expect(err).To(BeNil())

		Expect(fakeWriter.envs).To(HaveLen(1))
		Expect(fakeWriter.envs[0]).To(Equal(logEnv))
	})

	It("filters all envelope types except metric", func() {
		binding := syslog.Binding{AppId: "app-1", Hostname: "host-1", Drain: "syslog://drain.url.com", Type: syslog.BINDING_TYPE_METRIC}
		fakeWriter := &fakeWriter{}

		logEnv := &loggregator_v2.Envelope{
			Timestamp: time.Now().UnixNano(),
			SourceId:  "app-1",
			Message: &loggregator_v2.Envelope_Log{
				Log: &loggregator_v2.Log{
					Payload: []byte("hello"),
				},
			},
		}

		counterEnv := &loggregator_v2.Envelope{
			Timestamp: time.Now().UnixNano(),
			SourceId:  "app-1",
			Message: &loggregator_v2.Envelope_Counter{
				Counter: &loggregator_v2.Counter{
					Name:  "some-counter",
					Delta: 1,
					Total: 10,
				},
			},
		}

		gaugeEnv := &loggregator_v2.Envelope{
			Timestamp: time.Now().UnixNano(),
			SourceId:  "app-1",
			Message: &loggregator_v2.Envelope_Gauge{
				Gauge: &loggregator_v2.Gauge{},
			},
		}

		drain, err := syslog.NewFilteringDrainWriter(binding, fakeWriter)
		Expect(err).ToNot(HaveOccurred())

		err = drain.Write(logEnv)
		Expect(err).To(BeNil())

		err = drain.Write(counterEnv)
		Expect(err).To(BeNil())

		err = drain.Write(gaugeEnv)
		Expect(err).To(BeNil())

		Expect(fakeWriter.envs).To(HaveLen(2))
		Expect(fakeWriter.envs[0]).To(Equal(counterEnv))
		Expect(fakeWriter.envs[1]).To(Equal(gaugeEnv))
	})

	It("does no filtering for binding types of 'all' ", func() {
		binding := syslog.Binding{AppId: "app-1", Hostname: "host-1", Drain: "syslog://drain.url.com", Type: syslog.BINDING_TYPE_ALL}
		fakeWriter := &fakeWriter{}

		logEnv := &loggregator_v2.Envelope{
			Timestamp: time.Now().UnixNano(),
			SourceId:  "app-1",
			Message: &loggregator_v2.Envelope_Log{
				Log: &loggregator_v2.Log{
					Payload: []byte("hello"),
				},
			},
		}

		metricEnv := &loggregator_v2.Envelope{
			Timestamp: time.Now().UnixNano(),
			SourceId:  "app-1",
			Message: &loggregator_v2.Envelope_Counter{
				Counter: &loggregator_v2.Counter{
					Name:  "some-counter",
					Delta: 1,
					Total: 10,
				},
			},
		}

		drain, err := syslog.NewFilteringDrainWriter(binding, fakeWriter)
		Expect(err).ToNot(HaveOccurred())

		err = drain.Write(logEnv)
		Expect(err).To(BeNil())
		err = drain.Write(metricEnv)
		Expect(err).To(BeNil())

		Expect(fakeWriter.envs).To(HaveLen(2))
		Expect(fakeWriter.envs[0]).To(Equal(logEnv))
		Expect(fakeWriter.envs[1]).To(Equal(metricEnv))
	})

	DescribeTable("filters all envelopes that are not logs and metrics", func(env *loggregator_v2.Envelope) {
		binding := syslog.Binding{AppId: "app-1", Hostname: "host-1", Drain: "syslog://drain.url.com", Type: syslog.BINDING_TYPE_ALL}
		fakeWriter := &fakeWriter{}

		drain, err := syslog.NewFilteringDrainWriter(binding, fakeWriter)
		Expect(err).ToNot(HaveOccurred())

		err = drain.Write(env)
		Expect(err).To(BeNil())

		Expect(fakeWriter.envs).To(HaveLen(0))
	},
		Entry("timer", &loggregator_v2.Envelope{Message: &loggregator_v2.Envelope_Timer{Timer: &loggregator_v2.Timer{}}}),
		Entry("events", &loggregator_v2.Envelope{Message: &loggregator_v2.Envelope_Event{Event: &loggregator_v2.Event{}}}),
	)

	Describe("aggregate drains", func() {
		DescribeTable("allows all envelope types", func(env *loggregator_v2.Envelope) {
			binding := syslog.Binding{AppId: "all", Hostname: "host-1", Drain: "syslog://drain.url.com", Type: syslog.BINDING_TYPE_AGGREGATE}
			fakeWriter := &fakeWriter{}

			drain, err := syslog.NewFilteringDrainWriter(binding, fakeWriter)
			Expect(err).ToNot(HaveOccurred())

			err = drain.Write(env)
			Expect(err).To(BeNil())

			Expect(fakeWriter.envs).To(HaveLen(1))
		},
			Entry("log", &loggregator_v2.Envelope{Message: &loggregator_v2.Envelope_Log{Log: &loggregator_v2.Log{}}}),
			Entry("counter", &loggregator_v2.Envelope{Message: &loggregator_v2.Envelope_Counter{Counter: &loggregator_v2.Counter{}}}),
			Entry("gauge", &loggregator_v2.Envelope{Message: &loggregator_v2.Envelope_Gauge{Gauge: &loggregator_v2.Gauge{}}}),
			Entry("timer", &loggregator_v2.Envelope{Message: &loggregator_v2.Envelope_Timer{Timer: &loggregator_v2.Timer{}}}),
			Entry("events", &loggregator_v2.Envelope{Message: &loggregator_v2.Envelope_Event{Event: &loggregator_v2.Event{}}}),
		)
	})

	It("errors on invalid binding type", func() {
		binding := syslog.Binding{AppId: "app-1", Hostname: "host-1", Drain: "syslog://drain.url.com", Type: 10}
		_, err := syslog.NewFilteringDrainWriter(binding, &fakeWriter{})
		Expect(err).To(HaveOccurred())
	})
})

type fakeWriter struct {
	envs []*loggregator_v2.Envelope
}

func (f *fakeWriter) Write(env *loggregator_v2.Envelope) error {
	f.envs = append(f.envs, env)
	return nil
}
