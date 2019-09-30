package main

import (
	"code.cloudfoundry.org/go-metric-registry"
	"log"
	"os"

	"code.cloudfoundry.org/loggregator-agent/cmd/syslog-binding-cache/app"
)

func main() {
	logger := log.New(os.Stderr, "", log.LstdFlags)
	logger.Println("starting syslog-binding-cache")
	defer logger.Println("stopping syslog-binding-cache")

	cfg := app.LoadConfig()
	m := metrics.NewRegistry(
		logger,
		metrics.WithTLSServer(
			int(cfg.MetricsServer.Port),
			cfg.MetricsServer.CertFile,
			cfg.MetricsServer.KeyFile,
			cfg.MetricsServer.CAFile,
		),
	)
	app.NewSyslogBindingCache(cfg, m, logger).Run()
}
