package app

import (
	"log"
	"time"

	envstruct "code.cloudfoundry.org/go-envstruct"
)

type Config struct {
	// Loggregator Agent Certs
	ClientKeyPath  string `env:"CLIENT_KEY_PATH, report, required"`
	ClientCertPath string `env:"CLIENT_CERT_PATH, report, required"`
	CACertPath     string `env:"CA_CERT_PATH, report, required"`

	// System Metrics Agent Certs
	MetricsKeyPath    string `env:"METRICS_KEY_PATH, report, required"`
	MetricsCertPath   string `env:"METRICS_CERT_PATH, report, required"`
	MetricsCACertPath string `env:"METRICS_CA_CERT_PATH, report, required"`
	MetricsCN         string `env:"METRICS_CA_CN, report, required"`

	LoggregatorIngressAddr string `env:"LOGGREGATOR_AGENT_ADDR, report, required"`
	DefaultSourceID        string `env:"DEFAULT_SOURCE_ID, report, required"`

	ScrapeInterval time.Duration `env:"SCRAPE_INTERVAL, report"`
	ScrapeTimeout  time.Duration `env:"SCRAPE_TIMEOUT, report"`
	ScrapePort     int           `env:"SCRAPE_PORT, report, required"`
	DebugPort      int           `env:"DEBUG_PORT, report"`

	DNSFile              string `env:"DNS_FILE, report, required"`
	LeadershipServerAddr string `env:"LEADERSHIP_SERVER_ADDR, report"`
}

func LoadConfig(log *log.Logger) Config {
	cfg := Config{
		ScrapeInterval: time.Minute,
		ScrapeTimeout:  time.Second,
	}

	if err := envstruct.Load(&cfg); err != nil {
		log.Fatal(err)
	}

	envstruct.WriteReport(&cfg)

	return cfg
}
