package main

import (
	"log"
	"os"

	metrics "code.cloudfoundry.org/go-metric-registry"
	"code.cloudfoundry.org/loggregator-agent-release/src/cmd/prom-scraper/app"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/plumbing"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/scraper"
)

func main() {
	logger := log.New(os.Stderr, "", log.LstdFlags)
	logger.Printf("starting Prom Scraper...")
	defer logger.Printf("closing Prom Scraper...")

	cfg := app.LoadConfig(logger)
	if cfg.UseRFC339 {
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

	configProvider := scraper.NewConfigProvider(cfg.ConfigGlobs, cfg.DefaultScrapeInterval, logger).Configs
	app.NewPromScraper(cfg, configProvider, m, logger).Run()
}
