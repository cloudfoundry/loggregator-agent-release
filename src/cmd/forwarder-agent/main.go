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
		"origin": "loggregator_forwarder_agent",
		"source_id": "forwarder_agent",
	}

	metrics := metrics.NewRegistry(
		logger,
		metrics.WithDefaultTags(dt),
	)

	app.NewForwarderAgent(
		cfg,
		metrics,
		logger,
	).Run()
}
