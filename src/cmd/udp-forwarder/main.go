package main

import (
	"log"
	"os"

	metrics "code.cloudfoundry.org/go-metric-registry"

	_ "net/http/pprof"

	"code.cloudfoundry.org/loggregator-agent-release/src/cmd/udp-forwarder/app"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/plumbing"
)

func main() {
	logger := log.New(os.Stderr, "", log.LstdFlags)
	logger.Println("starting UDP Forwarder...")
	defer logger.Println("closing UDP Forwarder...")

	cfg := app.LoadConfig(logger)
	if cfg.UseRFC3339 {
		logger = log.New(new(plumbing.LogWriter), "", 0)
		logger.SetOutput(new(plumbing.LogWriter))
		logger.SetFlags(0)
	}

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
