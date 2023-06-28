package app

import (
	"code.cloudfoundry.org/go-envstruct"
)

type Cache struct {
	URL        string `env:"CACHE_URL,                 report"`
	CAFile     string `env:"CACHE_CA_FILE_PATH,        report"`
	CertFile   string `env:"CACHE_CERT_FILE_PATH,      report"`
	KeyFile    string `env:"CACHE_KEY_FILE_PATH,       report"`
	CommonName string `env:"CACHE_COMMON_NAME,         report"`
}

type Config struct {
	CollectorPidFile       string `env:"COLLECTOR_PID_FILE"`
	CollectorBaseConfig    string `env:"COLLECTOR_BASE_CONFIG"`
	CollectorRunningConfig string `env:"COLLECTOR_RUNNING_CONFIG"`
	CollectorBinary        string `env:"COLLECTOR_BINARY"`
	CollectorStdoutLog     string `env:"COLLECTOR_STDOUT_LOG"`
	CollectorStderrLog     string `env:"COLLECTOR_STDERR_LOG"`
	Cache                  Cache
}

func LoadConfig() (*Config, error) {
	cfg := Config{}
	err := envstruct.Load(&cfg)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}
