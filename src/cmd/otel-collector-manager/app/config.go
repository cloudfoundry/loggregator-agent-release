package app

import (
	"code.cloudfoundry.org/go-envstruct"
)

type Config struct {
	CollectorPidFile       string `env:"COLLECTOR_PID_FILE"`
	CollectorBaseConfig    string `env:"COLLECTOR_BASE_CONFIG"`
	CollectorRunningConfig string `env:"COLLECTOR_RUNNING_CONFIG"`
	CollectorBinary        string `env:"COLLECTOR_BINARY"`
	CollectorStdoutLog     string `env:"COLLECTOR_STDOUT_LOG"`
	CollectorStderrLog     string `env:"COLLECTOR_STDERR_LOG"`
}

func LoadConfig() (*Config, error) {
	cfg := Config{}
	err := envstruct.Load(&cfg)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}
