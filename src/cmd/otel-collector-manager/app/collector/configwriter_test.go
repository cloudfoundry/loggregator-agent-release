package collector_test

import (
	"os"
	"path/filepath"

	"code.cloudfoundry.org/loggregator-agent-release/src/cmd/otel-collector-manager/app"
	"code.cloudfoundry.org/loggregator-agent-release/src/cmd/otel-collector-manager/app/collector"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
)

var _ = Describe("ConfigWriter", func() {
	var (
		tempDirPath, baseFile, outputFile string

		cw *collector.ConfigWriter
	)

	BeforeEach(func() {
		var err error
		tempDirPath, err = os.MkdirTemp("", "otel-collector-manager-test")
		Expect(err).ToNot(HaveOccurred())

		baseConfig := `extensions:
  memory_ballast:
    size_mib: 512
service:
  pipelines:
    metrics:
      receivers: [file]`
		Expect(os.WriteFile(filepath.Join(tempDirPath, "base.yml"), []byte(baseConfig), 0600)).To(Succeed())

		baseFile = filepath.Join(tempDirPath, "base.yml")
		outputFile = filepath.Join(tempDirPath, "config.yml")
		cw = collector.NewConfigWriter(baseFile, outputFile)
	})

	AfterEach(func() {
		Expect(os.RemoveAll(tempDirPath)).To(Succeed())
	})

	Describe("Write", func() {
		It("includes the base configuration", func() {
			cfg := app.ExporterConfig{}
			Expect(cw.Write(cfg)).To(Succeed())

			b, err := os.ReadFile(outputFile)
			Expect(err).ToNot(HaveOccurred())
			var receivedCfg map[string]any
			err = yaml.Unmarshal(b, &receivedCfg)
			Expect(err).ToNot(HaveOccurred())
			Expect(receivedCfg["extensions"]).To(HaveKey("memory_ballast"))
		})

		It("sets the exporters section of the otel collector config", func() {
			cfg := app.ExporterConfig{
				"file": map[string]any{
					"path": "/some/path",
				},
				"prometheus": map[string]any{
					"endpoint": "localhost:9090",
				},
			}
			Expect(cw.Write(cfg)).To(Succeed())

			b, err := os.ReadFile(outputFile)
			Expect(err).ToNot(HaveOccurred())
			var receivedCfg map[string]any
			err = yaml.Unmarshal(b, &receivedCfg)
			Expect(err).ToNot(HaveOccurred())
			Expect(receivedCfg["exporters"]).To(BeEquivalentTo(cfg))
		})

		It("includes the provided exporters in the metrics pipeline", func() {
			cfg := app.ExporterConfig{
				"file": map[string]any{
					"path": "/some/path",
				},
				"prometheus": map[string]any{
					"endpoint": "localhost:9090",
				},
			}
			Expect(cw.Write(cfg)).To(Succeed())

			b, err := os.ReadFile(outputFile)
			Expect(err).ToNot(HaveOccurred())
			var receivedCfg collector.Config
			err = yaml.Unmarshal(b, &receivedCfg)
			Expect(err).ToNot(HaveOccurred())
			Expect(receivedCfg.Service.Pipelines.Metrics.Exporters).To(ConsistOf([]string{"file", "prometheus"}))
		})

		It("does not discard receivers configured in the metrics pipeline", func() {
			cfg := app.ExporterConfig{
				"file": map[string]any{
					"path": "/some/path",
				},
			}
			Expect(cw.Write(cfg)).To(Succeed())

			b, err := os.ReadFile(outputFile)
			Expect(err).ToNot(HaveOccurred())
			var receivedCfg collector.Config
			err = yaml.Unmarshal(b, &receivedCfg)
			Expect(err).ToNot(HaveOccurred())
			Expect(receivedCfg.Service.Pipelines.Metrics.Receivers).To(Equal([]string{"file"}))
		})

		Context("when the output file cannot be written to", func() {
			BeforeEach(func() {
				Expect(os.Mkdir(outputFile, 0700)).To(Succeed())
			})
			It("errors", func() {
				cw = collector.NewConfigWriter(baseFile, outputFile)
				err := cw.Write(app.ExporterConfig{})
				Expect(err).To(HaveOccurred())
			})
		})

		Context("when the base config file cannot be read", func() {
			BeforeEach(func() {
				Expect(os.Remove(baseFile)).To(Succeed())
			})
			It("errors", func() {
				cw = collector.NewConfigWriter(baseFile, outputFile)
				err := cw.Write(app.ExporterConfig{})
				Expect(err).To(HaveOccurred())
			})
		})

		Context("when the base config file is not well-formed YAML", func() {
			BeforeEach(func() {
				err := os.WriteFile(baseFile, []byte("not: well-formed: yaml"), 0600)
				Expect(err).ToNot(HaveOccurred())
			})
			It("errors", func() {
				cw = collector.NewConfigWriter(baseFile, outputFile)
				err := cw.Write(app.ExporterConfig{})
				Expect(err).To(HaveOccurred())
			})
		})

	})
})
