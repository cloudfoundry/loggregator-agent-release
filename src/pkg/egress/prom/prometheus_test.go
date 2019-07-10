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
	"time"
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

	It("drops metrics with invalid names", func(){
		promCollector := prom.NewCollector()
		Expect(promCollector.Write(gauge(map[string]float64{
			"gauge1.wrong.name": 11,
			"gauge2/also-wrong": 22,
		}))).ToNot(Succeed())

		Expect(promCollector.Write(totalCounter("counter.wrong.name", 11))).ToNot(Succeed())
		Expect(promCollector.Write(timer("timer.wrong.name", 11, 22))).ToNot(Succeed())

		Expect(collectMetrics(promCollector)).ToNot(Receive())
	})

	Context("envelope types", func() {
		It("collects counters from the provider", func() {
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

		It("collects gauges from the provider", func() {
			promCollector := prom.NewCollector()
			Expect(promCollector.Write(gauge(map[string]float64{
				"gauge1": 11,
				"gauge2": 22,
			}))).To(Succeed())

			Expect(collectMetrics(promCollector)).To(receiveInAnyOrder(
				And(
					haveName("gauge1"),
					gaugeWithValue(11),
				),
				And(
					haveName("gauge2"),
					gaugeWithValue(22),
				),
			))

			Expect(promCollector.Write(gauge(map[string]float64{
				"gauge1": 111,
				"gauge2": 222,
			}))).To(Succeed())

			Expect(collectMetrics(promCollector)).To(receiveInAnyOrder(
				And(
					haveName("gauge1"),
					gaugeWithValue(111),
				),
				And(
					haveName("gauge2"),
					gaugeWithValue(222),
				),
			))
		})

		It("collects timers from the provider", func() {
			promCollector := prom.NewCollector()
			Expect(promCollector.Write(timer("http", int64(time.Millisecond), int64(2*time.Millisecond)))).To(Succeed())

			Expect(collectMetrics(promCollector)).To(receiveInAnyOrder(
				And(
					haveName("http_seconds"),
					histogramWithCount(1),
					histogramWithSum(float64(time.Millisecond)/float64(time.Second)),
					histogramWithBuckets(0.01, 0.2, 1.0, 15.0, 60.0),
				),
			))

			Expect(promCollector.Write(timer("http", 0, int64(time.Second)))).To(Succeed())

			Expect(collectMetrics(promCollector)).To(receiveInAnyOrder(
				And(
					haveName("http_seconds"),
					histogramWithCount(2),
					histogramWithSum(float64(time.Second+time.Millisecond)/float64(time.Second)),
					histogramWithBuckets(0.01, 0.2, 1.0, 15.0, 60.0),
				),
			))
		})

		It("ignores unknown envelope types", func() {
			promCollector := prom.NewCollector()
			Expect(promCollector.Write(&loggregator_v2.Envelope{})).To(Succeed())
			Expect(collectMetrics(promCollector)).ToNot(Receive())
		})
	})

	Context("tags", func() {
		It("includes tags for counters", func() {
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

		It("includes tags for gauges", func() {
			promCollector := prom.NewCollector()
			gauge := gaugeWithUnit("some_gauge", "percentage")
			Expect(promCollector.Write(gauge)).To(Succeed())

			Expect(collectMetrics(promCollector)).To(Receive(And(
				haveName("some_gauge"),
				haveLabels(
					&dto.LabelPair{Name: proto.String("unit"), Value: proto.String("percentage")},
					&dto.LabelPair{Name: proto.String("source_id"), Value: proto.String("some-source-id")},
					&dto.LabelPair{Name: proto.String("instance_id"), Value: proto.String("some-instance-id")},
				),
			)))
		})

		It("includes tags for timers", func() {
			promCollector := prom.NewCollector()
			timer := timerWithTags("some_timer", map[string]string{
				"a": "1",
				"b": "2",
			})
			Expect(promCollector.Write(timer)).To(Succeed())

			Expect(collectMetrics(promCollector)).To(Receive(And(
				haveName("some_timer_seconds"),
				haveLabels(
					&dto.LabelPair{Name: proto.String("a"), Value: proto.String("1")},
					&dto.LabelPair{Name: proto.String("b"), Value: proto.String("2")},
					&dto.LabelPair{Name: proto.String("source_id"), Value: proto.String("some-source-id")},
					&dto.LabelPair{Name: proto.String("instance_id"), Value: proto.String("some-instance-id")},
				),
			)))
		})

		It("ignores units if empty", func() {
			promCollector := prom.NewCollector()
			Expect(promCollector.Write(gauge(map[string]float64{"some_gauge": 7}))).To(Succeed())

			Expect(collectMetrics(promCollector)).To(Receive(And(
				haveName("some_gauge"),
				haveLabels(
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

	It("differentiates between metrics with the same name and same tags but different source id", func() {
		promCollector := prom.NewCollector()
		counter := counterWithTags("some_counter", 1, map[string]string{
			"a": "1",
		})
		Expect(promCollector.Write(counter)).To(Succeed())

		sameCounter := counterWithTags("some_counter", 3, map[string]string{
			"a": "1",
		})
		Expect(promCollector.Write(sameCounter)).To(Succeed())

		counter.SourceId = "different_source_id"
		Expect(promCollector.Write(counter)).To(Succeed())

		Expect(collectMetrics(promCollector)).To(HaveLen(2))
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

func gaugeWithValue(val float64) types.GomegaMatcher {
	return WithTransform(func(metric prometheus.Metric) float64 {
		dtoMetric := &dto.Metric{}
		err := metric.Write(dtoMetric)
		Expect(err).ToNot(HaveOccurred())

		return dtoMetric.GetGauge().GetValue()
	}, Equal(val))
}

func histogramWithCount(count uint64) types.GomegaMatcher {
	return WithTransform(func(metric prometheus.Metric) uint64 {
		histogram := asHistogram(metric)
		return histogram.GetSampleCount()
	}, Equal(count))
}

func histogramWithBuckets(buckets ...float64) types.GomegaMatcher {
	return WithTransform(func(metric prometheus.Metric) []float64 {
		histogram := asHistogram(metric)
		var upperBounds []float64
		for _, bucket := range histogram.GetBucket() {
			upperBounds = append(upperBounds, bucket.GetUpperBound())
		}

		return upperBounds
	}, Equal(buckets))
}

func asHistogram(metric prometheus.Metric) *dto.Histogram {
	dtoMetric := &dto.Metric{}
	err := metric.Write(dtoMetric)
	Expect(err).ToNot(HaveOccurred())

	return dtoMetric.GetHistogram()
}

func histogramWithSum(sum float64) types.GomegaMatcher {
	return WithTransform(func(metric prometheus.Metric) float64 {
		dtoMetric := &dto.Metric{}
		err := metric.Write(dtoMetric)
		Expect(err).ToNot(HaveOccurred())

		histogram := dtoMetric.GetHistogram()
		return histogram.GetSampleSum()
	}, Equal(sum))
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

func timerWithTags(name string, tags map[string]string) *loggregator_v2.Envelope {
	return &loggregator_v2.Envelope{
		SourceId:   "some-source-id",
		InstanceId: "some-instance-id",
		Message: &loggregator_v2.Envelope_Timer{
			Timer: &loggregator_v2.Timer{
				Name:  name,
				Start: 0,
				Stop:  int64(time.Second),
			},
		},
		Tags: tags,
	}
}

func gauge(gauges map[string]float64) *loggregator_v2.Envelope {
	gaugeValues := map[string]*loggregator_v2.GaugeValue{}
	for name, value := range gauges {
		gaugeValues[name] = &loggregator_v2.GaugeValue{Value: value}
	}

	return &loggregator_v2.Envelope{
		SourceId:   "some-source-id",
		InstanceId: "some-instance-id",
		Message: &loggregator_v2.Envelope_Gauge{
			Gauge: &loggregator_v2.Gauge{
				Metrics: gaugeValues,
			},
		},
	}
}

func gaugeWithUnit(name, unit string) *loggregator_v2.Envelope {
	return &loggregator_v2.Envelope{
		SourceId:   "some-source-id",
		InstanceId: "some-instance-id",
		Message: &loggregator_v2.Envelope_Gauge{
			Gauge: &loggregator_v2.Gauge{
				Metrics: map[string]*loggregator_v2.GaugeValue{
					name: {
						Unit:  unit,
						Value: 1,
					},
				},
			},
		},
	}
}

func timer(name string, start, stop int64) *loggregator_v2.Envelope {
	return &loggregator_v2.Envelope{
		SourceId:   "some-source-id",
		InstanceId: "some-instance-id",
		Message: &loggregator_v2.Envelope_Timer{
			Timer: &loggregator_v2.Timer{
				Name:  name,
				Start: start,
				Stop:  stop,
			},
		},
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
