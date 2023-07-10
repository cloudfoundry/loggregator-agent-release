package syslog_test

import (
	"code.cloudfoundry.org/go-loggregator/v9/rpc/loggregator_v2"
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
})

type fakeWriter struct {
	received int
}

func (f *fakeWriter) Write(env *loggregator_v2.Envelope) error {
	f.received += 1
	return nil
}
