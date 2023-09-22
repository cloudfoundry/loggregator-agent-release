package otelcolclient

import (
	"time"

	"code.cloudfoundry.org/go-batching"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
)

// MetricBatcher batches OpenTelemetry Metrics.
type MetricBatcher struct {
	*batching.Batcher
	w MetricWriter
}

// MetricWriter is used to submit the completed batch of OpenTelemetry metrics. The
// batch may not be full if the interval lapsed instead of filling the batch.
type MetricWriter interface {
	// Write submits the batch.
	Write(batch []*metricspb.Metric)
	Close() error
}

// NewMetricBatcher creates a new OpenTelemetry Metric Batcher.
func NewMetricBatcher(size int, interval time.Duration, writer MetricWriter) *MetricBatcher {
	genWriter := batching.WriterFunc(func(batch []interface{}) {
		envBatch := make([]*metricspb.Metric, 0, len(batch))
		for _, element := range batch {
			envBatch = append(envBatch, element.(*metricspb.Metric))
		}
		writer.Write(envBatch)
	})
	return &MetricBatcher{
		Batcher: batching.NewBatcher(size, interval, genWriter),
		w:       writer,
	}
}

// Write stores data to the batch. It will not submit the batch to the writer
// until either the batch has been filled, or the interval has lapsed. NOTE:
// Write is *not* thread safe and should be called by the same goroutine that
// calls Flush.
func (b *MetricBatcher) Write(data *metricspb.Metric) {
	b.Batcher.Write(data)
}
