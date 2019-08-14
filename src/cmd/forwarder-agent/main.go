package main

import (
	"code.cloudfoundry.org/go-loggregator/metrics"
	"log"
	"os"

	"code.cloudfoundry.org/loggregator-agent/cmd/forwarder-agent/app"
)

func main() {
	logger := log.New(os.Stderr, "", log.LstdFlags)
	logger.Println("starting forwarder-agent")
	defer logger.Println("stopping forwarder-agent")

	cfg := app.LoadConfig()
	dt := map[string]string{
		"metrics_version": "2.0",
		"origin":          "loggregator_forwarder_agent",
	}

	m := metrics.NewRegistry(
		logger,
		metrics.WithDefaultTags(dt),
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
