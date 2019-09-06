package app

import (
	"code.cloudfoundry.org/loggregator-agent/pkg/config"
	"code.cloudfoundry.org/loggregator-agent/pkg/ingress/cups"
	"fmt"
	"time"

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
	URL             string               `env:"CACHE_URL,              required, report"`
	CAFile          string               `env:"CACHE_CA_FILE_PATH,     required, report"`
	CertFile        string               `env:"CACHE_CERT_FILE_PATH,   required, report"`
	KeyFile         string               `env:"CACHE_KEY_FILE_PATH,    required, report"`
	CommonName      string               `env:"CACHE_COMMON_NAME,      required, report"`
	PollingInterval time.Duration        `env:"CACHE_POLLING_INTERVAL, report"`
	Blacklist       cups.BlacklistRanges `env:"BLACKLISTED_SYSLOG_RANGES, report"`
}

type Capi struct {
	Endpoint     string `env:"CC_ENDPOINT, required, report"`
	ClientID     string `env:"CC_CLIENT_ID, required, report"`
	ClientSecret string `env:"CC_CLIENT_SECRET, required, report"`
}

type GroupCache struct {
	Peers    string `env:"PEERS, required, report"`
	CAFile   string `env:"GROUPCACHE_CA_FILE_PATH,     required, report"`
	CertFile string `env:"GROUPCACHE_CERT_FILE_PATH,   required, report"`
	KeyFile  string `env:"GROUPCACHE_KEY_FILE_PATH,    required, report"`
}

// Config holds the configuration for the syslog agent
type Config struct {
	BindingsPerAppLimit int           `env:"BINDING_PER_APP_LIMIT,    report"`
	DrainSkipCertVerify bool          `env:"DRAIN_SKIP_CERT_VERIFY,   report"`
	IdleDrainTimeout    time.Duration `env:"IDLE_DRAIN_TIMEOUT, report"`

	GroupCache    GroupCache
	GRPC          GRPC
	Cache         Cache
	Capi          Capi
	MetricsServer config.MetricsServer

	UniversalDrainURLs []string `env:"UNIVERSAL_DRAIN_URLS, report"`
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
	}
	if err := envstruct.Load(&cfg); err != nil {
		panic(fmt.Sprintf("Failed to load config from environment: %s", err))
	}

	envstruct.WriteReport(&cfg)

	return cfg
}
