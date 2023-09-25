package otelcolclient

import (
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
)

var _ = Describe("MetricBatcher", func() {
	It("batches metrics", func() {
		writer := &spyMetricWriter{}
		b := NewMetricBatcher(2, time.Minute, writer)
		b.Write(&metricspb.Metric{Name: "some.metric"})
		Expect(writer.BatchLen()).To(Equal(0))

		b.Write(&metricspb.Metric{Name: "another.metric"})
		Expect(writer.BatchLen()).To(Equal(2))
		Expect(writer.batch[0].Name).To(Equal("some.metric"))
		Expect(writer.batch[1].Name).To(Equal("another.metric"))
	})
	Context("when no writes have occurred for a while", func() {
		It("flushes pending writes", func() {
			writer := &spyMetricWriter{}
			b := NewMetricBatcher(1000, 10*time.Millisecond, writer)
			b.Write(&metricspb.Metric{Name: "some.metric"})
			Eventually(writer.BatchLen).Should(Equal(1))
			Expect(writer.batch[0].Name).To(Equal("some.metric"))
		})
	})
})

type spyMetricWriter struct {
	batch []*metricspb.Metric
	mu    sync.Mutex
}

func (w *spyMetricWriter) Write(batch []*metricspb.Metric) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.batch = batch
}

func (w *spyMetricWriter) BatchLen() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.batch)
}

func (w *spyMetricWriter) Close() error {
	return nil
}
