package syslog_test

import (
	"code.cloudfoundry.org/go-loggregator/v10/rpc/loggregator_v2"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/syslog"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Filtering Drain Writer", func() {
	DescribeTable("allows correct envelope types", func(drainData syslog.DrainData, logs, metrics, events, traces bool) {
		binding := syslog.Binding{Hostname: "host-1",
			Drain: syslog.Drain{
				Url: "syslog://drain.url.com",
			},
			DrainData: drainData}
		fakeWriter := &fakeWriter{}

		drain, err := syslog.NewFilteringDrainWriter(binding, fakeWriter)
		Expect(err).ToNot(HaveOccurred())
		length := 0
		envs := []*loggregator_v2.Envelope{
			{Message: &loggregator_v2.Envelope_Log{Log: &loggregator_v2.Log{}}},
			{Message: &loggregator_v2.Envelope_Counter{Counter: &loggregator_v2.Counter{}}},
			{Message: &loggregator_v2.Envelope_Gauge{Gauge: &loggregator_v2.Gauge{}}},
			{Message: &loggregator_v2.Envelope_Event{Event: &loggregator_v2.Event{}}},
			{Message: &loggregator_v2.Envelope_Timer{Timer: &loggregator_v2.Timer{}}},
		}
		shouldReceive := []bool{logs, metrics, metrics, events, traces}
		for index := range envs {
			err = drain.Write(envs[index])
			Expect(err).To(BeNil())
			if shouldReceive[index] {
				length += 1
			}
			Expect(length).To(Equal(fakeWriter.received))
		}
	},
		Entry("logs", syslog.LOGS, true, false, true, false),
		Entry("metrics", syslog.METRICS, false, true, false, false),
		Entry("traces", syslog.TRACES, false, false, false, true),
		Entry("all", syslog.ALL, true, true, true, true),
		Entry("log without events", syslog.LOGS_NO_EVENTS, true, false, false, false),
		Entry("metrics and logs", syslog.LOGS_AND_METRICS, true, true, false, false),
	)

	It("errors on invalid binding type", func() {
		binding := syslog.Binding{AppId: "app-1", Hostname: "host-1",
			Drain: syslog.Drain{
				Url: "syslog://drain.url.com",
			},
			DrainData: 10}
		_, err := syslog.NewFilteringDrainWriter(binding, &fakeWriter{})
		Expect(err).To(HaveOccurred())
	})

	It("sends logs when source_type tag is missing", func() {
		binding := syslog.Binding{
			DrainData: syslog.LOGS,
		}
		fakeWriter := &fakeWriter{}
		drainWriter, err := syslog.NewFilteringDrainWriter(binding, fakeWriter)
		Expect(err).NotTo(HaveOccurred())

		envelope := &loggregator_v2.Envelope{
			Message: &loggregator_v2.Envelope_Log{
				Log: &loggregator_v2.Log{
					Payload: []byte("test log"),
				},
			},
			Tags: map[string]string{
				// source_type tag is intentionally missing
			},
		}

		err = drainWriter.Write(envelope)

		Expect(err).NotTo(HaveOccurred())
		Expect(fakeWriter.received).To(Equal(1))
	})

	It("filters logs based on include filter - includes only APP logs", func() {
		appFilter := syslog.LogTypeSet{syslog.LOG_APP: struct{}{}}
		binding := syslog.Binding{
			DrainData: syslog.LOGS,
			LogFilter: &appFilter,
		}
		fakeWriter := &fakeWriter{}
		drainWriter, err := syslog.NewFilteringDrainWriter(binding, fakeWriter)
		Expect(err).NotTo(HaveOccurred())

		envelopes := []*loggregator_v2.Envelope{
			{
				Message: &loggregator_v2.Envelope_Log{
					Log: &loggregator_v2.Log{Payload: []byte("app log")},
				},
				Tags: map[string]string{"source_type": "APP/PROC/WEB/0"},
			},
			{
				Message: &loggregator_v2.Envelope_Log{
					Log: &loggregator_v2.Log{Payload: []byte("rtr log")},
				},
				Tags: map[string]string{"source_type": "RTR/1"},
			},
			{
				Message: &loggregator_v2.Envelope_Log{
					Log: &loggregator_v2.Log{Payload: []byte("stg log")},
				},
				Tags: map[string]string{"source_type": "STG/0"},
			},
		}

		for _, envelope := range envelopes {
			err = drainWriter.Write(envelope)
			Expect(err).NotTo(HaveOccurred())
		}

		// Only APP log should be sent
		Expect(fakeWriter.received).To(Equal(1))
	})

	It("filters logs based on exclude filter - excludes RTR logs", func() {
		// Include APP and STG, effectively excluding RTR
		includeFilter := syslog.LogTypeSet{
			syslog.LOG_APP: struct{}{},
			syslog.LOG_STG: struct{}{},
		}
		binding := syslog.Binding{
			DrainData: syslog.LOGS,
			LogFilter: &includeFilter,
		}
		fakeWriter := &fakeWriter{}
		drainWriter, err := syslog.NewFilteringDrainWriter(binding, fakeWriter)
		Expect(err).NotTo(HaveOccurred())

		envelopes := []*loggregator_v2.Envelope{
			{
				Message: &loggregator_v2.Envelope_Log{
					Log: &loggregator_v2.Log{Payload: []byte("app log")},
				},
				Tags: map[string]string{"source_type": "APP/PROC/WEB/0"},
			},
			{
				Message: &loggregator_v2.Envelope_Log{
					Log: &loggregator_v2.Log{Payload: []byte("rtr log")},
				},
				Tags: map[string]string{"source_type": "RTR/1"},
			},
			{
				Message: &loggregator_v2.Envelope_Log{
					Log: &loggregator_v2.Log{Payload: []byte("stg log")},
				},
				Tags: map[string]string{"source_type": "STG/0"},
			},
		}

		for _, envelope := range envelopes {
			err = drainWriter.Write(envelope)
			Expect(err).NotTo(HaveOccurred())
		}

		// APP and STG logs should be sent, RTR should be filtered out
		Expect(fakeWriter.received).To(Equal(2))
	})

	It("sends logs with unknown source_type prefix when filter is set", func() {
		appFilter := syslog.LogTypeSet{syslog.LOG_APP: struct{}{}}
		binding := syslog.Binding{
			DrainData: syslog.LOGS,
			LogFilter: &appFilter,
		}
		fakeWriter := &fakeWriter{}
		drainWriter, err := syslog.NewFilteringDrainWriter(binding, fakeWriter)
		Expect(err).NotTo(HaveOccurred())

		envelope := &loggregator_v2.Envelope{
			Message: &loggregator_v2.Envelope_Log{
				Log: &loggregator_v2.Log{
					Payload: []byte("test log"),
				},
			},
			Tags: map[string]string{
				"source_type": "UNKNOWN/some/path",
			},
		}

		err = drainWriter.Write(envelope)

		// Should send the log because unknown types default to being included
		Expect(err).NotTo(HaveOccurred())
		Expect(fakeWriter.received).To(Equal(1))
	})
})

type fakeWriter struct {
	received int
}

func (f *fakeWriter) Write(env *loggregator_v2.Envelope) error {
	f.received += 1
	return nil
}
