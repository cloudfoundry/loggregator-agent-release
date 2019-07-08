package main

import (
	"code.cloudfoundry.org/loggregator-agent/cmd/metrics-agent/app"
	"code.cloudfoundry.org/loggregator-agent/pkg/metrics"
	"log"
	_ "net/http/pprof"
	"os"
)

func main() {
	logger := log.New(os.Stderr, "", log.LstdFlags)
	logger.Println("starting metrics-agent")
	defer logger.Println("stopping metrics-agent")

	cfg := app.LoadConfig()

	m := metrics.NewPromRegistry(
		"metrics_agent",
		logger,
		metrics.WithServer(int(cfg.DebugPort)),
		metrics.WithDefaultTags(map[string]string{
			"metrics_version": "2.0",
			"origin": "loggregator.metrics_agent",
		}),
	)
	app.NewMetricsAgent(cfg, m, logger).Run()
}

