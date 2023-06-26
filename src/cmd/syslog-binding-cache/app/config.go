package app

import (
	"log"
	"time"

	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/config"

	envstruct "code.cloudfoundry.org/go-envstruct"
)

// Config holds the configuration for the syslog binding cache
type Config struct {
	UseRFC3339           bool          `env:"USE_RFC3339"`
	APIURL               string        `env:"API_URL,              required, report"`
	APICAFile            string        `env:"API_CA_FILE_PATH,     required, report"`
	APICertFile          string        `env:"API_CERT_FILE_PATH,   required, report"`
	APIKeyFile           string        `env:"API_KEY_FILE_PATH,    required, report"`
	APICommonName        string        `env:"API_COMMON_NAME,      required, report"`
	APIPollingInterval   time.Duration `env:"API_POLLING_INTERVAL, report"`
	APIBatchSize         int           `env:"API_BATCH_SIZE, report"`
	APIDisableKeepAlives bool          `env:"API_DISABLE_KEEP_ALIVES, report"`
	CipherSuites         []string      `env:"CIPHER_SUITES, report"`
	AggregateDrainsFile  string        `env:"AGGREGATE_DRAINS_FILE, report"`

	CacheCAFile     string `env:"CACHE_CA_FILE_PATH,     required, report"`
	CacheCertFile   string `env:"CACHE_CERT_FILE_PATH,   required, report"`
	CacheKeyFile    string `env:"CACHE_KEY_FILE_PATH,    required, report"`
	CacheCommonName string `env:"CACHE_COMMON_NAME,      required, report"`

	CachePort int `env:"CACHE_PORT, required, report"`

	MetricsServer config.MetricsServer
}

// LoadConfig will load the configuration for the syslog binding cache from the
// environment. If loading the config fails for any reason this function will
// panic.
func LoadConfig() Config {
	cfg := Config{
		APIPollingInterval: 15 * time.Second,
	}
	if err := envstruct.Load(&cfg); err != nil {
		log.Panicf("Failed to load config from environment: %s", err)
	}

	envstruct.WriteReport(&cfg) //nolint:errcheck

	return cfg
}
