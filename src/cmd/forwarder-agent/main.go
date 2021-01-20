package main

import (
	"log"
	"os"

	metrics "code.cloudfoundry.org/go-metric-registry"

	"code.cloudfoundry.org/loggregator-agent-release/src/cmd/forwarder-agent/app"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/plumbing"
)

func main() {
	logger := log.New(os.Stderr, "", log.LstdFlags)
	logger.Println("starting forwarder-agent")
	defer logger.Println("stopping forwarder-agent")

	cfg := app.LoadConfig()
	if cfg.UseRFC3339 {
		logger = log.New(new(plumbing.LogWriter), "", 0)
		logger.SetOutput(new(plumbing.LogWriter))
		logger.SetFlags(0)
	}
	m := metrics.NewRegistry(
		logger,
		metrics.WithTLSServer(
			int(cfg.MetricsServer.Port),
			cfg.MetricsServer.CertFile,
			cfg.MetricsServer.KeyFile,
			cfg.MetricsServer.CAFile,
		),
	)

	app.NewForwarderAgent(
		cfg,
		m,
		logger,
	).Run()
}
