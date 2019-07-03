package main

import (
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

	app.NewMetricsAgent(cfg, logger).Run()
}

