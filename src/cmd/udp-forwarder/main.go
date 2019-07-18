package main

import (
	"code.cloudfoundry.org/go-loggregator/metrics"
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
		metrics.WithDefaultTags(map[string]string{
			"origin":    "loggregator.udp_forwarder",
			"source_id": "udp_forwarder",
		}),
	)

	forwarder := app.NewUDPForwarder(cfg, logger, m)
	forwarder.Run()
}
