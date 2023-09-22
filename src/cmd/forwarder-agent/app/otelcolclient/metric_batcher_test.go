package otelcolclient

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
)

var _ = Describe("MetricBatcher", func() {
	It("batches metrics", func() {
		writer := &spyMetricWriter{}
		b := NewMetricBatcher(1, time.Minute, writer)
		b.Write(&metricspb.Metric{Name: "some.metric"})

		Expect(writer.batch).To(HaveLen(1))
		Expect(writer.batch[0].Name).To(Equal("some.metric"))
	})
})

type spyMetricWriter struct {
	batch []*metricspb.Metric
}

func (w *spyMetricWriter) Write(batch []*metricspb.Metric) {
	w.batch = batch
}

func (w *spyMetricWriter) Close() error {
	return nil
}
