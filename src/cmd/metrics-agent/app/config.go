package app

import (
	"code.cloudfoundry.org/go-envstruct"
	"fmt"
	"time"
)

// GRPCConfig stores the configuration for the router as a server using a PORT
// with mTLS certs.
type GRPCConfig struct {
	Port     uint16 `env:"AGENT_PORT, report"`
	CAFile   string `env:"AGENT_CA_FILE_PATH, required, report"`
	CertFile string `env:"AGENT_CERT_FILE_PATH, required, report"`
	KeyFile  string `env:"AGENT_KEY_FILE_PATH, required, report"`
}

// MetricsConfig stores the configuration for the metrics server using a PORT
// with mTLS certs.
type MetricsConfig struct {
	Port                 uint16            `env:"METRICS_PORT, required, report"`
	CAFile               string            `env:"METRICS_CA_FILE_PATH, required, report"`
	CertFile             string            `env:"METRICS_CERT_FILE_PATH, required, report"`
	KeyFile              string            `env:"METRICS_KEY_FILE_PATH, required, report"`
	WhitelistedTimerTags []string          `env:"WHITELISTED_TIMER_TAGS, required, report"`
	DefaultTags          map[string]string `env:"AGENT_TAGS"`

	ExpirationInterval time.Duration `env:"EXPIRATION_INTERVAL, report"`
	TimeToLive         time.Duration `env:"TTL, report"`
}

// Config holds the configuration for the metrics agent
type Config struct {
	Metrics   MetricsConfig
	GRPC      GRPCConfig
	Tags      map[string]string `env:"AGENT_TAGS"`
	DebugPort uint16            `env:"DEBUG_PORT, required, report"`
}

// LoadConfig will load the configuration for the forwarder agent from the
// environment. If loading the config fails for any reason this function will
// panic.
func LoadConfig() Config {
	cfg := Config{
		GRPC: GRPCConfig{
			Port: 3458,
		},
		Metrics: MetricsConfig{
			TimeToLive:         10 * time.Minute,
			ExpirationInterval: time.Minute,
		},
	}
	if err := envstruct.Load(&cfg); err != nil {
		panic(fmt.Sprintf("Failed to load config from environment: %s", err))
	}

	envstruct.WriteReport(&cfg)

	return cfg
}
