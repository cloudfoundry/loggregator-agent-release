package config

// MetricsServer stores the configuration for the metrics server
type MetricsServer struct {
	DebugMetrics bool   `env:"DEBUG_METRICS, report"`
	Port         uint16 `env:"METRICS_PORT, report"`
	PprofPort    uint16 `env:"PPROF_PORT, report"`
	CAFile       string `env:"METRICS_CA_FILE_PATH, required, report"`
	CertFile     string `env:"METRICS_CERT_FILE_PATH, required, report"`
	KeyFile      string `env:"METRICS_KEY_FILE_PATH, required, report"`
}
