package syslog

import (
	"log"

	"code.cloudfoundry.org/go-loggregator/v10/rpc/loggregator_v2"
	metrics "code.cloudfoundry.org/go-metric-registry"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress"
)

type drainGetter func(sourceID string) []egress.Writer
type nextEnvelope func() *loggregator_v2.Envelope

type EnvelopeWriter struct {
	drainGetter  drainGetter
	nextEnvelope nextEnvelope
	ingress      metrics.Counter
	log          *log.Logger
}

func NewEnvelopeWriter(drainGetter drainGetter, nextEnvelope nextEnvelope, ingress metrics.Counter, log *log.Logger) *EnvelopeWriter {
	return &EnvelopeWriter{
		drainGetter:  drainGetter,
		nextEnvelope: nextEnvelope,
		ingress:      ingress,
		log:          log,
	}
}

func (w *EnvelopeWriter) Run() {
	for {
		envelope := w.nextEnvelope()
		w.writeEnvelope(envelope)
	}
}

func (w *EnvelopeWriter) writeEnvelope(envelope *loggregator_v2.Envelope) {
	drains := w.drainGetter(envelope.GetSourceId())
	for _, drain := range drains {
		w.ingress.Add(1)
		err := drain.Write(envelope)
		if err != nil {
			w.log.Print(err)
		}
	}
}
