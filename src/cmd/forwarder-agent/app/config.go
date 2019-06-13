package app

import (
	"fmt"

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
	// DownstreamIngressPortCfg will define consumers on localhost that will
	// receive each envelope. It is assumed to adhere to the Loggregator Ingress
	// Service and use the provided TLS configuration.
	DownstreamIngressPortCfg string `env:"DOWNSTREAM_INGRESS_PORT_GLOB, report"`
	DebugPort                uint16 `env:"DEBUG_PORT, report"`
	GRPC                     GRPC
	Tags                     map[string]string `env:"AGENT_TAGS"`
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

	envstruct.WriteReport(&cfg)

	return cfg
}
