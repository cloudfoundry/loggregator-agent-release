package main

import (
	"code.cloudfoundry.org/loggregator-agent/cmd/prom-scraper/app"
	"log"
	"os"
)

func main() {
	log := log.New(os.Stderr, "", log.LstdFlags)
	log.Printf("starting Prom Scraper...")
	defer log.Printf("closing Prom Scraper...")

	cfg := app.LoadConfig(log)
	app.NewPromScraper(cfg, log).Run()
}
