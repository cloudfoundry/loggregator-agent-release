package main

import (
	"code.cloudfoundry.org/go-loggregator/metrics"
	"log"
	"os"

	"code.cloudfoundry.org/loggregator-agent/cmd/syslog-binding-cache/app"
)

func main() {
	logger := log.New(os.Stderr, "", log.LstdFlags)
	logger.Println("starting syslog-binding-cache")
	defer logger.Println("stopping syslog-binding-cache")

	cfg := app.LoadConfig()
	m := metrics.NewRegistry(
		logger,
		metrics.WithServer(int(cfg.DebugPort)),
		metrics.WithDefaultTags(map[string]string{
			"origin": "loggregator_syslog_binding_cache",
			"source_id": "syslog_binding_cache",
		}),
	)
	app.NewSyslogBindingCache(cfg, m, logger).Run()
}
