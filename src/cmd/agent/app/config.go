package app

import (
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/config"
	"fmt"
	"strings"

	"code.cloudfoundry.org/go-envstruct"
	"golang.org/x/net/idna"
)

// GRPC stores the configuration for the router as a server using a PORT
// with mTLS certs and as a client.
type GRPC struct {
	Port         uint16   `env:"AGENT_PORT"`
	CAFile       string   `env:"AGENT_CA_FILE"`
	CertFile     string   `env:"AGENT_CERT_FILE"`
	KeyFile      string   `env:"AGENT_KEY_FILE"`
	CipherSuites []string `env:"AGENT_CIPHER_SUITES"`
}

// Config stores all configurations options for the Agent.
type Config struct {
	Deployment                      string            `env:"AGENT_DEPLOYMENT"`
	Zone                            string            `env:"AGENT_ZONE"`
	Job                             string            `env:"AGENT_JOB"`
	Index                           string            `env:"AGENT_INDEX"`
	IP                              string            `env:"AGENT_IP"`
	Tags                            map[string]string `env:"AGENT_TAGS"`
	DisableUDP                      bool              `env:"AGENT_DISABLE_UDP"`
	IncomingUDPPort                 int               `env:"AGENT_INCOMING_UDP_PORT"`
	MetricBatchIntervalMilliseconds uint              `env:"AGENT_METRIC_BATCH_INTERVAL_MILLISECONDS"`
	MetricSourceID                  string            `env:"AGENT_METRIC_SOURCE_ID"`
	RouterAddr                      string            `env:"ROUTER_ADDR"`
	RouterAddrWithAZ                string            `env:"ROUTER_ADDR_WITH_AZ"`
	GRPC                            GRPC
	MetricsServer                   config.MetricsServer
}

// LoadConfig reads from the environment to create a Config.
func LoadConfig() (*Config, error) {
	cfg := Config{
		MetricBatchIntervalMilliseconds: 60000,
		MetricSourceID:                  "metron",
		IncomingUDPPort:                 3457,
		MetricsServer: config.MetricsServer{
			Port: 14824,
		},
		GRPC: GRPC{
			Port: 3458,
		},
	}
	err := envstruct.Load(&cfg)
	if err != nil {
		return nil, err
	}

	if cfg.RouterAddr == "" {
		return nil, fmt.Errorf("RouterAddr is required")
	}

	cfg.RouterAddrWithAZ, err = idna.ToASCII(cfg.RouterAddrWithAZ)
	if err != nil {
		return nil, err
	}
	cfg.RouterAddrWithAZ = strings.Replace(cfg.RouterAddrWithAZ, "@", "-", -1)

	return &cfg, nil
}
