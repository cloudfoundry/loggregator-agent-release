package collector

import (
	"os"

	"code.cloudfoundry.org/loggregator-agent-release/src/cmd/otel-collector-manager/app"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Exporters map[string]any
	Service   struct {
		Pipelines struct {
			Metrics struct {
				Receivers []string
				Exporters []string
			}
		}
	}
	Other map[string]any `yaml:",inline"`
}

type ConfigWriter struct {
	baseFile   string
	outputFile string
}

func NewConfigWriter(baseFile, outputFile string) *ConfigWriter {
	return &ConfigWriter{
		baseFile:   baseFile,
		outputFile: outputFile,
	}
}

func (cw *ConfigWriter) Write(exporterCfg app.ExporterConfig) error {
	data, err := os.ReadFile(cw.baseFile)
	if err != nil {
		return err
	}

	cfg := Config{}
	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		return err
	}

	cfg.Exporters = exporterCfg
	cfg.Service.Pipelines.Metrics.Exporters = keys(exporterCfg)

	data, err = yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	err = os.WriteFile(cw.outputFile, data, 0600)
	if err != nil {
		return err
	}

	return nil
}

func keys(m map[string]any) []string {
	i := 0
	ks := make([]string, len(m))
	for k := range m {
		ks[i] = k
		i++
	}
	return ks
}
