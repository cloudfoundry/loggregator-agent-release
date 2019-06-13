package app

import (
	"log"
	"time"

	"code.cloudfoundry.org/go-envstruct"
)

type Config struct {
	// Loggregator Agent Certs
	ClientKeyPath  string `env:"CLIENT_KEY_PATH, report, required"`
	ClientCertPath string `env:"CLIENT_CERT_PATH, report, required"`
	CACertPath     string `env:"CA_CERT_PATH, report, required"`

	LoggregatorIngressAddr string        `env:"LOGGREGATOR_AGENT_ADDR, report, required"`
	DefaultSourceID        string        `env:"DEFAULT_SOURCE_ID, report, required"`
	MetricPortCfg          string        `env:"METRIC_PORT_GLOB, report"`
	ScrapeInterval         time.Duration `env:"SCRAPE_INTERVAL, report"`
}

func LoadConfig(log *log.Logger) Config {
	cfg := Config{
		ScrapeInterval: 15 * time.Second,
	}

	if err := envstruct.Load(&cfg); err != nil {
		log.Fatal(err)
	}

	envstruct.WriteReport(&cfg)

	return cfg
}
