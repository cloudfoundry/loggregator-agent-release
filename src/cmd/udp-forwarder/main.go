package main

import (
	"log"
	"os"

	_ "net/http/pprof"

	"code.cloudfoundry.org/loggregator-agent/cmd/udp-forwarder/app"
	"code.cloudfoundry.org/loggregator-agent/pkg/metrics"
)

func main() {
	logger := log.New(os.Stderr, "", log.LstdFlags)
	logger.Println("starting UDP Forwarder...")
	defer logger.Println("closing UDP Forwarder...")

	cfg := app.LoadConfig(logger)
	m := metrics.NewPromRegistry("udp_forwarder", logger)

	forwarder := app.NewUDPForwarder(cfg, logger, m)
	forwarder.Run()
}
