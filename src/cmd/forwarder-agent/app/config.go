package app

import (
	"fmt"

	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/config"

	"code.cloudfoundry.org/go-envstruct"
)

// GRPC stores the configuration for the router as a server using a PORT
// with mTLS certs and as a client.
type GRPC struct {
	Port         uint16   `env:"AGENT_PORT, report"`
	CAFile       string   `env:"AGENT_CA_FILE_PATH, required, report"`
	CertFile     string   `env:"AGENT_CERT_FILE_PATH, required, report"`
	KeyFile      string   `env:"AGENT_KEY_FILE_PATH, required, report"`
	CipherSuites []string `env:"AGENT_CIPHER_SUITES, report"`
}

// Config holds the configuration for the forwarder agent
type Config struct {
	UseRFC3339 bool `env:"USE_RFC3339"`
	// DownstreamIngressPortCfg will define consumers on localhost that will
	// receive each envelope. It is assumed to adhere to the Loggregator Ingress
	// Service and use the provided TLS configuration.
	DownstreamIngressPortCfg string `env:"DOWNSTREAM_INGRESS_PORT_GLOB, report"`
	GRPC                     GRPC
	MetricsServer            config.MetricsServer
	Tags                     map[string]string `env:"AGENT_TAGS"`
	DebugMetrics             bool              `env:"DEBUG_METRICS, report"`
}

// LoadConfig will load the configuration for the forwarder agent from the
// environment. If loading the config fails for any reason this function will
// panic.
func LoadConfig() Config {
	cfg := Config{
		GRPC: GRPC{
			Port: 3458,
		},
	}
	if err := envstruct.Load(&cfg); err != nil {
		panic(fmt.Sprintf("Failed to load config from environment: %s", err))
	}

	if err := envstruct.WriteReport(&cfg); err != nil {
		panic(fmt.Sprintf("Failed to print a report of the from environment: %s", err))
	}

	return cfg
}
