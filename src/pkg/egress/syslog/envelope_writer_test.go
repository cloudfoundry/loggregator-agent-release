package syslog_test

import (
	"code.cloudfoundry.org/go-loggregator/v10/rpc/loggregator_v2"
	metricsHelpers "code.cloudfoundry.org/go-metric-registry/testhelpers"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/syslog"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("EnvelopeWriter", func() {
	It("writes envelopes to the drains associated with the source id", func() {
		spyWriter1 := newSpyWriter()
		spyWriter2 := newSpyWriter()
		expectedSourceID := "some-source-id"
		drainGetter := func(sourceID string) []egress.Writer {
			if sourceID != expectedSourceID {
				return nil
			}

			return []egress.Writer{spyWriter1, spyWriter2}
		}

		envelope := &loggregator_v2.Envelope{
			SourceId: expectedSourceID,
		}
		nextEnvelope := func() *loggregator_v2.Envelope {
			return envelope
		}

		writer := syslog.NewEnvelopeWriter(drainGetter, nextEnvelope, &metricsHelpers.SpyMetric{}, nil)

		go writer.Run()
		Eventually(spyWriter1.envelopes).Should(Receive(Equal(envelope)))
		Eventually(spyWriter2.envelopes).Should(Receive(Equal(envelope)))
	})

	It("writes envelopes until stopped", func() {
		spyWriter := newSpyWriter()
		expectedSourceID := "some-source-id"
		drainGetter := func(string) []egress.Writer {
			return []egress.Writer{spyWriter}
		}

		nextEnvelope := func() *loggregator_v2.Envelope {
			return &loggregator_v2.Envelope{
				SourceId: expectedSourceID,
			}
		}

		writer := syslog.NewEnvelopeWriter(drainGetter, nextEnvelope, &metricsHelpers.SpyMetric{}, nil)

		go writer.Run()
		Eventually(spyWriter.envelopes).Should(HaveLen(100))
	})

	It("increments the ingress metric", func() {
		spyWriter := newSpyWriter()
		expectedSourceID := "some-source-id"
		drainGetter := func(string) []egress.Writer {
			return []egress.Writer{spyWriter}
		}

		var callCount int
		nextEnvelope := func() *loggregator_v2.Envelope {
			callCount++
			if callCount > 2 {
				<-make(chan struct{})
			}
			return &loggregator_v2.Envelope{
				SourceId: expectedSourceID,
			}
		}

		ingressMetric := &metricsHelpers.SpyMetric{}
		writer := syslog.NewEnvelopeWriter(drainGetter, nextEnvelope, ingressMetric, nil)

		go writer.Run()
		Eventually(ingressMetric.Value).Should(BeNumerically("==", 2))
	})
})

type spyWriter struct {
	envelopes chan *loggregator_v2.Envelope
}

func newSpyWriter() *spyWriter {
	return &spyWriter{
		envelopes: make(chan *loggregator_v2.Envelope, 100),
	}
}

func (w *spyWriter) Write(env *loggregator_v2.Envelope) error {
	w.envelopes <- env
	return nil
}
