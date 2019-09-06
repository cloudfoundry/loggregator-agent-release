package syslog

import (
	"code.cloudfoundry.org/go-loggregator/metrics"
	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	"code.cloudfoundry.org/loggregator-agent/pkg/egress"
	"fmt"
	"log"
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

type cacheInterface interface {
	Get(guid string) string
}

func (w *EnvelopeWriter) Run(appCache cacheInterface) {
	for {
		envelope := w.nextEnvelope()
		w.writeEnvelope(envelope, appCache)
	}
}

func (w *EnvelopeWriter) writeEnvelope(envelope *loggregator_v2.Envelope, appCache cacheInterface) {
	drains := w.drainGetter(envelope.GetSourceId())
	for _, drain := range drains {
		w.ingress.Add(1)

		if envelope.Tags["origin"] == "gorouter" && envelope.GetSourceId() != "gorouter" {
			_, ok := envelope.Tags["space_name"]
			if !ok {
				spaceName := appCache.Get(envelope.GetSourceId())
				fmt.Printf("adding space name (%s) for: %s", spaceName, envelope.GetSourceId())
				envelope.Tags["space_name"] = spaceName
			}
		}

		err := drain.Write(envelope)
		if err != nil {
			w.log.Print(err)
		}
	}
}
