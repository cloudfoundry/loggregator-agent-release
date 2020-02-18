package v2

import (
	"code.cloudfoundry.org/go-metric-registry"
	"time"

	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/plumbing/batching"
)

type Nexter interface {
	TryNext() (*loggregator_v2.Envelope, bool)
}

type BatchWriter interface {
	Write(msgs []*loggregator_v2.Envelope) error
}

type Transponder struct {
	nexter        Nexter
	writer        BatchWriter
	batchSize     int
	batchInterval time.Duration
	droppedMetric metrics.Counter
	egressMetric  metrics.Counter
}

type MetricClient interface {
	NewCounter(name, helpText string, opts ...metrics.MetricOption) metrics.Counter
}

func NewTransponder(
	n Nexter,
	w BatchWriter,
	batchSize int,
	batchInterval time.Duration,
	metricClient MetricClient,
) *Transponder {
	droppedMetric := metricClient.NewCounter(
		"dropped",
		"Total number of dropped envelopes.",
		metrics.WithMetricLabels(map[string]string{"direction": "egress", "metric_version": "2.0"}),
	)
	egressMetric := metricClient.NewCounter(
		"egress",
		"Total number of envelopes successfully egressed.",
		metrics.WithMetricLabels(map[string]string{"metric_version": "2.0"}),
	)
	return &Transponder{
		nexter:        n,
		writer:        w,
		droppedMetric: droppedMetric,
		egressMetric:  egressMetric,
		batchSize:     batchSize,
		batchInterval: batchInterval,
	}
}

func (t *Transponder) Start() {
	b := batching.NewV2EnvelopeBatcher(
		t.batchSize,
		t.batchInterval,
		batching.V2EnvelopeWriterFunc(t.write),
	)

	for {
		envelope, ok := t.nexter.TryNext()
		if !ok {
			b.Flush()
			time.Sleep(100 * time.Millisecond)
			continue
		}

		b.Write(envelope)
	}
}

func (t *Transponder) write(batch []*loggregator_v2.Envelope) {
	if err := t.writer.Write(batch); err != nil {
		// metric-documentation-v2: (loggregator.metron.dropped) Number of messages
		// dropped when failing to write to Dopplers v2 API
		t.droppedMetric.Add(float64(len(batch)))
		return
	}

	// metric-documentation-v2: (loggregator.metron.egress)
	// Number of messages written to Doppler's v2 API
	t.egressMetric.Add(float64(len(batch)))
}
