package main

import (
	"code.cloudfoundry.org/loggregator-agent/cmd/metrics-agent/app"
	"code.cloudfoundry.org/go-loggregator/metrics"
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
		metrics.WithTLSServer(int(cfg.DebugPort), cfg.Metrics.CertFile, cfg.Metrics.KeyFile, cfg.Metrics.CAFile),
	)
	app.NewMetricsAgent(cfg, m, logger).Run()
}

