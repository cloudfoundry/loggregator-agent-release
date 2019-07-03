package app

import (
	"code.cloudfoundry.org/go-envstruct"
	"fmt"
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
	Port     uint16 `env:"METRICS_PORT, report"`
	CAFile   string `env:"METRICS_CA_FILE_PATH, required, report"`
	CertFile string `env:"METRICS_CERT_FILE_PATH, required, report"`
	KeyFile  string `env:"METRICS_KEY_FILE_PATH, required, report"`
}

// Config holds the configuration for the metrics agent
type Config struct {
	Metrics MetricsConfig
	GRPC    GRPCConfig
	Tags    map[string]string `env:"AGENT_TAGS"`
}

// LoadConfig will load the configuration for the forwarder agent from the
// environment. If loading the config fails for any reason this function will
// panic.
func LoadConfig() Config {
	cfg := Config{
		GRPC: GRPCConfig{
			Port: 3458,
		},
	}
	if err := envstruct.Load(&cfg); err != nil {
		panic(fmt.Sprintf("Failed to load config from environment: %s", err))
	}

	envstruct.WriteReport(&cfg)

	return cfg
}
