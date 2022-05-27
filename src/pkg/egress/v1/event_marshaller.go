package v1

import (
	"log"
	"sync"

	metrics "code.cloudfoundry.org/go-metric-registry"

	"github.com/cloudfoundry/sonde-go/events"
	"github.com/gogo/protobuf/proto"
)

//go:generate hel --type BatchChainByteWriter --output mock_writer_test.go

// MetricClient creates new CounterMetrics to be emitted periodically.
type MetricClient interface {
	NewCounter(name, helpText string, opts ...metrics.MetricOption) metrics.Counter
}

type BatchChainByteWriter interface {
	Write(message []byte) (err error)
}

type EventMarshaller struct {
	egressCounter func(uint64)
	byteWriter    BatchChainByteWriter
	bwLock        sync.RWMutex
}

func NewMarshaller(mc MetricClient) *EventMarshaller {
	egressMetric := mc.NewCounter(
		"egress",
		"Total number of envelopes successfully egressed.",
		metrics.WithMetricLabels(map[string]string{"metric_version": "1.0"}),
	)
	return &EventMarshaller{
		egressCounter: func(i uint64) { egressMetric.Add(float64(i)) },
	}
}

func (m *EventMarshaller) SetWriter(byteWriter BatchChainByteWriter) {
	m.bwLock.Lock()
	defer m.bwLock.Unlock()
	m.byteWriter = byteWriter
}

func (m *EventMarshaller) writer() BatchChainByteWriter {
	m.bwLock.RLock()
	defer m.bwLock.RUnlock()
	return m.byteWriter
}

func (m *EventMarshaller) Write(envelope *events.Envelope) {
	writer := m.writer()
	if writer == nil {
		log.Print("EventMarshaller: Write called while byteWriter is nil")
		return
	}

	envelopeBytes, err := proto.Marshal(envelope)
	if err != nil {
		log.Printf("marshalling error: %v", err)
		return
	}

	writer.Write(envelopeBytes)
	m.egressCounter(1)
}
