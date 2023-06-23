package metricbinding_test

import (
	"os"
	"path/filepath"

	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/metricbinding"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Store", func() {

	var tempDirPath, drainConfigPath, drains string

	BeforeEach(func() {
		var err error
		tempDirPath, err = os.MkdirTemp("", "aggregate-metric-drain")
		Expect(err).ToNot(HaveOccurred())

		drains = `---
otlp:
  endpoint: otelcol:4317
otlp/2:
  endpoint: otelcol2:4317
`
	})

	AfterEach(func() {
		Expect(os.RemoveAll(tempDirPath)).To(Succeed())
	})

	It("can read aggregate metric drains from a file", func() {
		drainConfigPath = filepath.Join(tempDirPath, "aggregate_metric_drains.yml")
		Expect(os.WriteFile(drainConfigPath, []byte(drains), 0600)).To(Succeed())

		store, err := metricbinding.NewAggregateStore(drainConfigPath)
		Expect(err).ToNot(HaveOccurred())

		Expect(store.Get()).To(BeEquivalentTo(
			map[string]any{
				"otlp": metricbinding.OtelExporterConfig{
					"endpoint": "otelcol:4317",
				},
				"otlp/2": metricbinding.OtelExporterConfig{
					"endpoint": "otelcol2:4317",
				},
			},
		))
	})

	Context("when the drain config cannot be read", func() {
		It("errors", func() {
			_, err := metricbinding.NewAggregateStore(drainConfigPath)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("when the drain config is malformed", func() {
		BeforeEach(func() {
			drains = "invalid: yaml: content"

			drainConfigPath = filepath.Join(tempDirPath, "aggregate_metric_drains.yml")
			Expect(os.WriteFile(drainConfigPath, []byte(drains), 0600)).To(Succeed())
		})
		It("errors", func() {
			_, err := metricbinding.NewAggregateStore(drainConfigPath)
			Expect(err).To(HaveOccurred())
		})
	})

})
