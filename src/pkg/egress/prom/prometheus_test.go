package prom_test

import (
	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	"code.cloudfoundry.org/loggregator-agent/pkg/egress/prom"
	"fmt"
	"github.com/gogo/protobuf/proto"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

var _ = Describe("Collector", func() {
	It("collects all received metrics", func() {
		promCollector := prom.NewCollector()
		Expect(promCollector.Write(totalCounter("metric1", 22))).To(Succeed())
		Expect(promCollector.Write(totalCounter("metric2", 22))).To(Succeed())

		metrics := collectMetrics(promCollector)
		Expect(metrics).To(HaveLen(2))
		Expect([]prometheus.Metric{<-metrics, <-metrics}).To(ConsistOf(
			haveName("metric1"),
			haveName("metric2"),
		))

		Expect(collectMetrics(promCollector)).To(receiveInAnyOrder(
			haveName("metric1"),
			haveName("metric2"),
		))
	})

	It("collects counters with totals from the provider", func() {
		promCollector := prom.NewCollector()
		Expect(promCollector.Write(totalCounter("some_total_counter", 22))).To(Succeed())

		Expect(collectMetrics(promCollector)).To(Receive(And(
			haveName("some_total_counter"),
			counterWithValue(22),
		)))

		Expect(promCollector.Write(totalCounter("some_total_counter", 37))).To(Succeed())
		Expect(collectMetrics(promCollector)).To(Receive(And(
			haveName("some_total_counter"),
			counterWithValue(37),
		)))
	})

	It("ignores non counters", func() {
		promCollector := prom.NewCollector()
		Expect(promCollector.Write(&loggregator_v2.Envelope{})).To(Succeed())
		Expect(collectMetrics(promCollector)).ToNot(Receive())
	})

	Context("tags", func() {

		It("includes tags", func() {
			promCollector := prom.NewCollector()
			counter := counterWithTags("label_counter", 1, map[string]string{
				"a": "1",
				"b": "2",
			})
			Expect(promCollector.Write(counter)).To(Succeed())

			Expect(collectMetrics(promCollector)).To(Receive(And(
				haveName("label_counter"),
				haveLabels(
					&dto.LabelPair{Name: proto.String("a"), Value: proto.String("1")},
					&dto.LabelPair{Name: proto.String("b"), Value: proto.String("2")},
					&dto.LabelPair{Name: proto.String("source_id"), Value: proto.String("some-source-id")},
					&dto.LabelPair{Name: proto.String("instance_id"), Value: proto.String("some-instance-id")},
				),
			)))
		})

		It("ignores invalid tags", func() {
			promCollector := prom.NewCollector()
			counter := counterWithTags("label_counter", 1, map[string]string{
				"__invalid":    "1",
				"not.valid":    "2",
				"totally_fine": "3",
			})
			Expect(promCollector.Write(counter)).To(Succeed())

			Expect(collectMetrics(promCollector)).To(Receive(And(
				haveName("label_counter"),
				haveLabels(
					&dto.LabelPair{Name: proto.String("totally_fine"), Value: proto.String("3")},
					&dto.LabelPair{Name: proto.String("source_id"), Value: proto.String("some-source-id")},
					&dto.LabelPair{Name: proto.String("instance_id"), Value: proto.String("some-instance-id")},
				),
			)))
		})

		It("ignores tags with empty values", func() {
			promCollector := prom.NewCollector()
			counter := counterWithTags("label_counter", 1, map[string]string{
				"a": "1",
				"b": "2",
				"c": "",
			})
			Expect(promCollector.Write(counter)).To(Succeed())

			Expect(collectMetrics(promCollector)).To(Receive(And(
				haveName("label_counter"),
				haveLabels(
					&dto.LabelPair{Name: proto.String("a"), Value: proto.String("1")},
					&dto.LabelPair{Name: proto.String("b"), Value: proto.String("2")},
					&dto.LabelPair{Name: proto.String("source_id"), Value: proto.String("some-source-id")},
					&dto.LabelPair{Name: proto.String("instance_id"), Value: proto.String("some-instance-id")},
				),
			)))
		})

		It("does not include instance_id if empty", func() {
			promCollector := prom.NewCollector()
			counter := counterWithEmptyInstanceID("some_name", 1)
			Expect(promCollector.Write(counter)).To(Succeed())

			Expect(collectMetrics(promCollector)).To(Receive(And(
				haveName("some_name"),
				haveLabels(
					&dto.LabelPair{Name: proto.String("source_id"), Value: proto.String("some-source-id")},
				),
			)))
		})
	})

	It("differentiates between metrics with the same name but different labels", func() {
		promCollector := prom.NewCollector()
		counter1 := counterWithTags("some_counter", 1, map[string]string{
			"a": "1",
		})
		counter2 := counterWithTags("some_counter", 2, map[string]string{
			"a": "2",
		})
		counter3 := counterWithTags("some_counter", 3, map[string]string{
			"a": "1",
			"b": "2",
		})
		Expect(promCollector.Write(counter1)).To(Succeed())
		Expect(promCollector.Write(counter2)).To(Succeed())
		Expect(promCollector.Write(counter3)).To(Succeed())

		Expect(collectMetrics(promCollector)).To(receiveInAnyOrder(
			counterWithValue(1),
			counterWithValue(2),
			counterWithValue(3),
		))
	})
})

func receiveInAnyOrder(elements ...interface{}) types.GomegaMatcher {
	return WithTransform(func(metricChan chan prometheus.Metric) []prometheus.Metric {
		close(metricChan)
		var metricSlice []prometheus.Metric
		for metric := range metricChan {
			metricSlice = append(metricSlice, metric)
		}

		return metricSlice
	}, ConsistOf(elements...))
}

func collectMetrics(promCollector *prom.Collector) chan prometheus.Metric {
	collectedMetrics := make(chan prometheus.Metric, 10)
	promCollector.Collect(collectedMetrics)
	return collectedMetrics
}

func counterWithValue(val float64) types.GomegaMatcher {
	return WithTransform(func(metric prometheus.Metric) float64 {
		dtoMetric := &dto.Metric{}
		err := metric.Write(dtoMetric)
		Expect(err).ToNot(HaveOccurred())

		return dtoMetric.GetCounter().GetValue()
	}, Equal(val))
}

func haveLabels(labels ...interface{}) types.GomegaMatcher {
	return WithTransform(func(metric prometheus.Metric) []*dto.LabelPair {
		dtoMetric := &dto.Metric{}
		err := metric.Write(dtoMetric)
		Expect(err).ToNot(HaveOccurred())

		return dtoMetric.GetLabel()
	}, ConsistOf(labels...))
}

func haveName(name string) types.GomegaMatcher {
	return WithTransform(func(metric prometheus.Metric) string {
		return metric.Desc().String()
	}, ContainSubstring(fmt.Sprintf(`fqName: "%s"`, name)))
}

func totalCounter(name string, total uint64) *loggregator_v2.Envelope {
	return &loggregator_v2.Envelope{
		SourceId:   "some-source-id",
		InstanceId: "some-instance-id",
		Message: &loggregator_v2.Envelope_Counter{
			Counter: &loggregator_v2.Counter{
				Name:  name,
				Total: total,
			},
		},
	}
}

func counterWithTags(name string, total uint64, tags map[string]string) *loggregator_v2.Envelope {
	return &loggregator_v2.Envelope{
		SourceId:   "some-source-id",
		InstanceId: "some-instance-id",
		Message: &loggregator_v2.Envelope_Counter{
			Counter: &loggregator_v2.Counter{
				Name:  name,
				Total: total,
			},
		},
		Tags: tags,
	}
}

func counterWithEmptyInstanceID(name string, total uint64) *loggregator_v2.Envelope {
	return &loggregator_v2.Envelope{
		SourceId: "some-source-id",
		Message: &loggregator_v2.Envelope_Counter{
			Counter: &loggregator_v2.Counter{
				Name:  name,
				Total: total,
			},
		},
	}
}
