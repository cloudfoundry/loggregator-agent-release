package app

import (
	"fmt"
	"time"

	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/config"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/ingress/cups"

	"code.cloudfoundry.org/go-envstruct"
)

// GRPC stores the configuration for the router as a server using a PORT
// with mTLS certs and as a client.
type GRPC struct {
	Port         int      `env:"AGENT_PORT, report"`
	CAFile       string   `env:"AGENT_CA_FILE_PATH, required, report"`
	CertFile     string   `env:"AGENT_CERT_FILE_PATH, required, report"`
	KeyFile      string   `env:"AGENT_KEY_FILE_PATH, required, report"`
	CipherSuites []string `env:"AGENT_CIPHER_SUITES, report"`
}

type Cache struct {
	URL             string               `env:"CACHE_URL,              report"`
	CAFile          string               `env:"CACHE_CA_FILE_PATH,      report"`
	CertFile        string               `env:"CACHE_CERT_FILE_PATH,    report"`
	KeyFile         string               `env:"CACHE_KEY_FILE_PATH,     report"`
	CommonName      string               `env:"CACHE_COMMON_NAME,       report"`
	PollingInterval time.Duration        `env:"CACHE_POLLING_INTERVAL, report"`
	Blacklist       cups.BlacklistRanges `env:"BLACKLISTED_SYSLOG_RANGES, report"`
}

// Config holds the configuration for the syslog agent
type Config struct {
	BindingsPerAppLimit  int    `env:"BINDING_PER_APP_LIMIT,    report"`
	DrainSkipCertVerify  bool   `env:"DRAIN_SKIP_CERT_VERIFY,   report"`
	DrainTrustedCAFile   string `env:"DRAIN_TRUSTED_CA_FILE, report"`
	DefaultDrainMetadata bool   `env:"DEFAULT_DRAIN_METADATA", report"`

	IdleDrainTimeout time.Duration `env:"IDLE_DRAIN_TIMEOUT, report"`

	GRPC          GRPC
	Cache         Cache
	MetricsServer config.MetricsServer

	AggregateConnectionRefreshInterval time.Duration `env:"AGGREGATE_CONNECTION_REFRESH_INTERVAL, report"`
	AggregateDrainURLs                 []string      `env:"AGGREGATE_DRAIN_URLS, report"`
}

// LoadConfig will load the configuration for the syslog agent from the
// environment. If loading the config fails for any reason this function will
// panic.
func LoadConfig() Config {
	cfg := Config{
		BindingsPerAppLimit: 5,
		IdleDrainTimeout:    10 * time.Minute,

		Cache: Cache{
			PollingInterval: 1 * time.Minute,
		},
		GRPC: GRPC{
			Port: 3458,
		},
		DefaultDrainMetadata:               true,
		AggregateConnectionRefreshInterval: 1 * time.Minute,
	}
	if err := envstruct.Load(&cfg); err != nil {
		panic(fmt.Sprintf("Failed to load config from environment: %s", err))
	}

	envstruct.WriteReport(&cfg)

	return cfg
}
