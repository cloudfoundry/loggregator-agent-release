package otelcolclient

import (
	"sync"
	"time"

	"code.cloudfoundry.org/go-batching"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// SignalBatcher batches OpenTelemetry signals.
type SignalBatcher struct {
	metricsBatcher, traceBatcher *batching.Batcher
	w                            Writer
	mu                           sync.Mutex
}

// Writer is used to submit the completed batches of OpenTelemetry signals. The
// batch may not be full if the interval lapsed instead of filling the batch.
type Writer interface {
	// WriteMetrics submits the batch.
	WriteMetrics(batch []*metricspb.Metric)
	// WriteTrace submits the batch.
	WriteTrace(batch []*tracepb.ResourceSpans)
	Close() error
}

// NewSignalBatcher creates a new OpenTelemetry Metric Batcher.
func NewSignalBatcher(size int, interval time.Duration, writer Writer) *SignalBatcher {
	metricsWriter := batching.WriterFunc(func(batch []any) {
		envBatch := make([]*metricspb.Metric, 0, len(batch))
		for _, element := range batch {
			envBatch = append(envBatch, element.(*metricspb.Metric))
		}
		writer.WriteMetrics(envBatch)
	})
	traceWriter := batching.WriterFunc(func(batch []any) {
		envBatch := make([]*tracepb.ResourceSpans, 0, len(batch))
		for _, element := range batch {
			envBatch = append(envBatch, element.(*tracepb.ResourceSpans))
		}
		writer.WriteTrace(envBatch)
	})

	sb := &SignalBatcher{
		metricsBatcher: batching.NewBatcher(size, interval, metricsWriter),
		traceBatcher:   batching.NewBatcher(size, interval, traceWriter),
		w:              writer,
	}
	go func() {
		for {
			time.Sleep(interval)
			sb.Flush()
		}
	}()
	return sb
}

// WriteMetric stores data to the metric batch. It will not submit the batch to
// the writer until either the batch has been filled, or the interval has
// lapsed.
func (b *SignalBatcher) WriteMetric(data *metricspb.Metric) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.metricsBatcher.Write(data)
}

// WriteTrace stores data to the trace batch. It will not submit the batch to
// the writer until either the batch has been filled, or the interval has
// lapsed.
func (b *SignalBatcher) WriteTrace(data *tracepb.ResourceSpans) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.traceBatcher.Write(data)
}

// Flush will write a partial batch if there is data and the interval has
// lapsed. Otherwise it is a NOP. This method should be called freqently to
// make sure batches do not stick around for long periods of time. As a result
// it would be a bad idea to call Flush after an operation that might block
// for an un-specified amount of time.
func (b *SignalBatcher) Flush() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.metricsBatcher.Flush()
	b.traceBatcher.Flush()
}
