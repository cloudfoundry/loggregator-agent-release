package main

import (
	"code.cloudfoundry.org/go-loggregator/metrics"
	"code.cloudfoundry.org/loggregator-agent/cmd/metrics-agent/app"
	"log"
	_ "net/http/pprof"
	"os"
)

func main() {
	logger := log.New(os.Stderr, "", log.LstdFlags)
	logger.Println("starting metrics-agent")
	defer logger.Println("stopping metrics-agent")

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
	app.NewMetricsAgent(cfg, m, logger).Run()
}
