package app

import (
	"code.cloudfoundry.org/loggregator-agent/pkg/config"
	"log"
	"time"

	"code.cloudfoundry.org/go-envstruct"
)

type Config struct {
	// Loggregator Agent Certs
	ClientKeyPath  string `env:"CLIENT_KEY_PATH, report, required"`
	ClientCertPath string `env:"CLIENT_CERT_PATH, report, required"`
	CACertPath     string `env:"CA_CERT_PATH, report, required"`

	// Prom Scraper Certs
	ScrapeKeyPath    string `env:"SCRAPE_KEY_PATH, report"`
	ScrapeCertPath   string `env:"SCRAPE_CERT_PATH, report"`
	ScrapeCACertPath string `env:"SCRAPE_CA_CERT_PATH, report"`

	LoggregatorIngressAddr string        `env:"LOGGREGATOR_AGENT_ADDR, report, required"`
	DefaultSourceID        string        `env:"DEFAULT_SOURCE_ID, report, required"`
	ConfigGlobs            []string      `env:"CONFIG_GLOBS, report"`
	DefaultScrapeInterval  time.Duration `env:"SCRAPE_INTERVAL, report"`
	SkipSSLValidation      bool          `env:"SKIP_SSL_VALIDATION, report"`

	MetricsServer config.MetricsServer
}

func LoadConfig(log *log.Logger) Config {
	cfg := Config{
		DefaultScrapeInterval: 15 * time.Second,
	}

	if err := envstruct.Load(&cfg); err != nil {
		log.Fatal(err)
	}

	envstruct.WriteReport(&cfg)

	return cfg
}
