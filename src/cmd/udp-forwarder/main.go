package main

import (
	"code.cloudfoundry.org/go-metric-registry"
	"log"
	"os"

	_ "net/http/pprof"

	"code.cloudfoundry.org/loggregator-agent/cmd/udp-forwarder/app"
)

func main() {
	logger := log.New(os.Stderr, "", log.LstdFlags)
	logger.Println("starting UDP Forwarder...")
	defer logger.Println("closing UDP Forwarder...")

	cfg := app.LoadConfig(logger)
	m := metrics.NewRegistry(logger,
		metrics.WithTLSServer(
			int(cfg.MetricsServer.Port),
			cfg.MetricsServer.CertFile,
			cfg.MetricsServer.KeyFile,
			cfg.MetricsServer.CAFile,
		),
	)

	forwarder := app.NewUDPForwarder(cfg, logger, m)
	forwarder.Run()
}
