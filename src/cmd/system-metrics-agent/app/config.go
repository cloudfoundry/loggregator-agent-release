package app

import (
	"log"
	"time"

	envstruct "code.cloudfoundry.org/go-envstruct"
)

// Config holds the configuration for the system metrics agent.
type Config struct {
	SampleInterval time.Duration `env:"SAMPLE_INTERVAL,            report"`
	Deployment     string        `env:"DEPLOYMENT, report"`
	Job            string        `env:"JOB, report"`
	Index          string        `env:"INDEX, report"`
	IP             string        `env:"IP, report"`

	DebugPort  uint16 `env:"DEBUG_PORT, report"`
	MetricPort uint16 `env:"METRIC_PORT, report, required"`

	CACertPath string `env:"CA_CERT_PATH, required, report"`
	CertPath   string `env:"CERT_PATH, required, report"`
	KeyPath    string `env:"KEY_PATH, required, report"`
}

func LoadConfig() Config {
	cfg := Config{
		SampleInterval: time.Minute,
		MetricPort:     0,
	}

	if err := envstruct.Load(&cfg); err != nil {
		log.Panicf("failed to load config from environment: %s", err)
	}

	_ = envstruct.WriteReport(&cfg)

	return cfg
}
