package otelcolclient

import (
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

var _ = Describe("SignalBatcher", func() {
	It("batches metrics", func() {
		writer := &spySignalWriter{}
		b := NewSignalBatcher(2, time.Minute, writer)
		b.WriteMetric(&metricspb.Metric{Name: "some.metric"})
		Expect(writer.MetricsBatchLen()).To(Equal(0))

		b.WriteMetric(&metricspb.Metric{Name: "another.metric"})
		Expect(writer.MetricsBatchLen()).To(Equal(2))
		Expect(writer.metrics[0].Name).To(Equal("some.metric"))
		Expect(writer.metrics[1].Name).To(Equal("another.metric"))
	})

	It("batches traces", func() {
		writer := &spySignalWriter{}
		b := NewSignalBatcher(2, time.Minute, writer)
		b.WriteTrace(&tracepb.ResourceSpans{})
		Expect(writer.TraceBatchLen()).To(Equal(0))

		b.WriteTrace(&tracepb.ResourceSpans{})
		Expect(writer.TraceBatchLen()).To(Equal(2))
	})

	Context("when no writes have occurred for a while", func() {
		It("flushes pending writes", func() {
			writer := &spySignalWriter{}
			b := NewSignalBatcher(1000, 10*time.Millisecond, writer)
			b.WriteMetric(&metricspb.Metric{Name: "some.metric"})
			b.WriteTrace(&tracepb.ResourceSpans{ScopeSpans: []*tracepb.ScopeSpans{{Spans: []*tracepb.Span{{TraceId: []byte("test")}}}}})
			Eventually(writer.MetricsBatchLen).Should(Equal(1))
			Expect(writer.metrics[0].Name).To(Equal("some.metric"))
			Eventually(writer.TraceBatchLen()).Should(Equal(1))
			Expect(writer.trace[0].ScopeSpans[0].Spans[0].TraceId).To(Equal([]byte("test")))
		})
	})
})

type spySignalWriter struct {
	metrics []*metricspb.Metric
	trace   []*tracepb.ResourceSpans
	mu      sync.Mutex
}

func (w *spySignalWriter) WriteMetrics(batch []*metricspb.Metric) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.metrics = batch
}

func (w *spySignalWriter) WriteTrace(batch []*tracepb.ResourceSpans) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.trace = batch
}

func (w *spySignalWriter) MetricsBatchLen() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.metrics)
}

func (w *spySignalWriter) TraceBatchLen() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.trace)
}

func (w *spySignalWriter) Close() error {
	return nil
}
