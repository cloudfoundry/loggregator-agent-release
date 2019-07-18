package main

import (
	"code.cloudfoundry.org/go-loggregator/metrics"
	"log"
	"os"

	"code.cloudfoundry.org/loggregator-agent/cmd/syslog-agent/app"
)

func main() {
	log := log.New(os.Stderr, "", log.LstdFlags)
	log.Println("starting syslog-agent")
	defer log.Println("stopping syslog-agent")

	cfg := app.LoadConfig()
	m := metrics.NewRegistry(
		log,
		metrics.WithDefaultTags(map[string]string{
			"metrics_version": "2.0",
			"origin": "loggregator.syslog_agent",
			"source_id": "syslog_agent",
		}),
	)

	app.NewSyslogAgent(cfg, m, log).Run()
}
