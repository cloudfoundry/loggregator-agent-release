package main

import (
	"code.cloudfoundry.org/loggregator-agent/pkg/metrics"
	"log"
	"os"

	"code.cloudfoundry.org/loggregator-agent/cmd/metric-scraper/app"
)

func main() {
	log := log.New(os.Stderr, "", log.LstdFlags)
	log.Printf("starting Metrics Scraper...")
	defer log.Printf("closing Metrics Scraper...")

	cfg := app.LoadConfig(log)

	dt := map[string]string{
		"metrics_version": "2.0",
	}
	metricClient := metrics.NewPromRegistry(
		"metric_scraper",
		log,
		metrics.WithDefaultTags(dt),
		metrics.WithServer(cfg.DebugPort),
	)

	app.NewMetricScraper(cfg, log, metricClient).Run()
}
