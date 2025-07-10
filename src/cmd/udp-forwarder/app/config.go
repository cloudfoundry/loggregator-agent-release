package app

import (
	"log"

	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/config"

	envstruct "code.cloudfoundry.org/go-envstruct"
)

// GRPC stores the configuration for the UDP agent to connect to the
// loggregator agent via gRPC over mTLS.
type GRPC struct {
	Addr     string `env:"LOGGREGATOR_AGENT_ADDR, report"`
	CAFile   string `env:"LOGGREGATOR_AGENT_CA_FILE_PATH, required, report"`
	CertFile string `env:"LOGGREGATOR_AGENT_CERT_FILE_PATH, required, report"`
	KeyFile  string `env:"LOGGREGATOR_AGENT_KEY_FILE_PATH, required, report"`
}

// Config holds the configuration for the UDP agent
type Config struct {
	UseRFC3339           bool   `env:"USE_RFC3339"`
	Host                 string `env:"AGENT_HOST, report"`
	UDPPort              int    `env:"UDP_PORT, report"`
	LoggregatorAgentGRPC GRPC
	Deployment           string `env:"DEPLOYMENT, report"`
	Job                  string `env:"JOB, report"`
	Index                string `env:"INDEX, report"`
	IP                   string `env:"IP, report"`

	MetricsServer config.MetricsServer
}

// LoadConfig reads from the environment to create a Config.
func LoadConfig(log *log.Logger) Config {
	cfg := Config{
		Host:    "127.0.0.1",
		UDPPort: 3457,
		LoggregatorAgentGRPC: GRPC{
			Addr: "127.0.0.1:3458",
		},
	}

	err := envstruct.Load(&cfg)
	if err != nil {
		log.Fatal(err)
	}

	envstruct.WriteReport(&cfg) //nolint:errcheck

	return cfg
}
