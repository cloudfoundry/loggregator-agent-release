package stats_test

import (
	"code.cloudfoundry.org/loggregator-agent/pkg/egress/stats"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
)

var _ = Describe("Prometheus Registry", func() {
	var (
		registry   *stats.PromRegistry
		registerer *stubRegisterer
	)

	BeforeEach(func() {
		registerer = newStubRegisterer()
		registry = stats.NewPromRegistry(registerer)
	})

	It("registers the gauge when it's first gotten", func() {
		gauge := toPromGauge(registry.Get("metric_name", "origin", "unit", nil))
		Expect(gauge.Desc().String()).To(Equal(`Desc{fqName: "metric_name", help: "vm metric", constLabels: {unit="unit"}, variableLabels: []}`))

		var registered prometheus.Gauge
		Eventually(registerer.gauges).Should(Receive(&registered))

		Expect(registered.Desc().String()).To(Equal(`Desc{fqName: "metric_name", help: "vm metric", constLabels: {unit="unit"}, variableLabels: []}`))
	})

	It("doesn't reregister gauges", func() {
		gauge := toPromGauge(registry.Get("metric_name", "origin", "unit", nil))
		Expect(gauge.Desc().String()).To(Equal(`Desc{fqName: "metric_name", help: "vm metric", constLabels: {unit="unit"}, variableLabels: []}`))

		gauge = toPromGauge(registry.Get("metric_name", "origin", "unit", nil))
		Expect(gauge.Desc().String()).To(Equal(`Desc{fqName: "metric_name", help: "vm metric", constLabels: {unit="unit"}, variableLabels: []}`))

		Expect(registerer.gauges).To(HaveLen(1))
	})

	It("gauges with different tags are different gauges", func() {
		gauge := toPromGauge(registry.Get("metric_name", "origin", "unit", map[string]string{"foo":"bar2"}))
		Expect(gauge.Desc().String()).To(Equal(`Desc{fqName: "metric_name", help: "vm metric", constLabels: {foo="bar2",unit="unit"}, variableLabels: []}`))

		gauge = toPromGauge(registry.Get("metric_name", "origin", "unit", map[string]string{"foo": "bar"}))
		Expect(gauge.Desc().String()).To(Equal(`Desc{fqName: "metric_name", help: "vm metric", constLabels: {foo="bar",unit="unit"}, variableLabels: []}`))

		Expect(registerer.gauges).To(HaveLen(2))
	})
})

func toPromGauge(g stats.Gauge) prometheus.Gauge {
	gauge, ok := g.(prometheus.Gauge)
	Expect(ok).To(BeTrue())
	return gauge
}

type stubRegisterer struct {
	gauges chan prometheus.Gauge
}

func newStubRegisterer() *stubRegisterer {
	return &stubRegisterer{
		gauges: make(chan prometheus.Gauge, 100),
	}
}

func (r *stubRegisterer) Register(c prometheus.Collector) error {
	gauge, ok := c.(prometheus.Gauge)
	Expect(ok).To(BeTrue())

	r.gauges <- gauge
	return nil
}

func (r *stubRegisterer) MustRegister(...prometheus.Collector) {

}

func (r *stubRegisterer) Unregister(prometheus.Collector) bool {
	return false
}
