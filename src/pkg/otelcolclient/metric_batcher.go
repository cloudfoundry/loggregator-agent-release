package otelcolclient

import (
	"sync"
	"time"

	"code.cloudfoundry.org/go-batching"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
)

// MetricBatcher batches OpenTelemetry Metrics.
type MetricBatcher struct {
	*batching.Batcher
	w  MetricWriter
	mu sync.Mutex
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
	mb := &MetricBatcher{
		Batcher: batching.NewBatcher(size, interval, genWriter),
		w:       writer,
	}
	go func() {
		for {
			time.Sleep(interval)
			mb.Flush()
		}
	}()
	return mb
}

// Write stores data to the batch. It will not submit the batch to the writer
// until either the batch has been filled, or the interval has lapsed.
func (b *MetricBatcher) Write(data *metricspb.Metric) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Batcher.Write(data)
}

// ForcedFlush bypasses the batch interval and batch size checks and writes
// immediately.
func (b *MetricBatcher) ForcedFlush() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Batcher.ForcedFlush()
}

// Flush will write a partial batch if there is data and the interval has
// lapsed. Otherwise it is a NOP. This method should be called freqently to
// make sure batches do not stick around for long periods of time. As a result
// it would be a bad idea to call Flush after an operation that might block
// for an un-specified amount of time.
func (b *MetricBatcher) Flush() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Batcher.Flush()
}
