package v2

import (
	"log"

	metrics "code.cloudfoundry.org/go-metric-registry"

	"code.cloudfoundry.org/go-loggregator/v9/rpc/loggregator_v2"
	"golang.org/x/net/context"
)

type DataSetter interface {
	Set(e *loggregator_v2.Envelope)
}

// MetricClient creates new CounterMetrics to be emitted periodically.
type MetricClient interface {
	NewCounter(name, helpText string, opts ...metrics.MetricOption) metrics.Counter
}

type Receiver struct {
	loggregator_v2.UnimplementedIngressServer

	dataSetter           DataSetter
	ingressMetric        func(uint64)
	originMappingsMetric func(uint64)
}

func NewReceiver(setter DataSetter, ingress metrics.Counter, egress metrics.Counter) *Receiver {
	return &Receiver{
		dataSetter:           setter,
		ingressMetric:        func(i uint64) { ingress.Add(float64(i)) },
		originMappingsMetric: func(i uint64) { egress.Add(float64(i)) },
	}
}

func (s *Receiver) Sender(sender loggregator_v2.Ingress_SenderServer) error {
	for {
		e, err := sender.Recv()
		if err != nil {
			log.Printf("Failed to receive data: %s", err)
			return err
		}
		e.SourceId = s.sourceID(e)
		s.dataSetter.Set(e)
		s.ingressMetric(1)
	}
}

func (s *Receiver) BatchSender(sender loggregator_v2.Ingress_BatchSenderServer) error {
	for {
		envelopes, err := sender.Recv()
		if err != nil {
			log.Printf("Failed to receive data: %s", err)
			return err
		}

		for _, e := range envelopes.Batch {
			e.SourceId = s.sourceID(e)
			s.dataSetter.Set(e)
		}
		s.ingressMetric(uint64(len(envelopes.Batch)))
	}
}

func (s *Receiver) Send(_ context.Context, b *loggregator_v2.EnvelopeBatch) (*loggregator_v2.SendResponse, error) {
	for _, e := range b.Batch {
		e.SourceId = s.sourceID(e)
		s.dataSetter.Set(e)
	}

	s.ingressMetric(uint64(len(b.Batch)))

	return &loggregator_v2.SendResponse{}, nil
}

func (r *Receiver) sourceID(e *loggregator_v2.Envelope) string {
	if e.SourceId != "" {
		return e.SourceId
	}

	if id, ok := e.GetTags()["origin"]; ok {
		r.originMappingsMetric(1)
		return id
	}

	if id, ok := e.GetDeprecatedTags()["origin"]; ok {
		r.originMappingsMetric(1)
		return id.GetText()
	}

	return ""
}
