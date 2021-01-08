package main

import (
	"log"
	"os"

	metrics "code.cloudfoundry.org/go-metric-registry"

	"code.cloudfoundry.org/loggregator-agent-release/src/cmd/syslog-binding-cache/app"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/plumbing"
)

func main() {
	logger := log.New(os.Stderr, "", log.LstdFlags)
	logger.Println("starting syslog-binding-cache")
	defer logger.Println("stopping syslog-binding-cache")

	cfg := app.LoadConfig()
	if cfg.UseRFC339 {
		logger = log.New(new(plumbing.LogWriter), "", 0)
		log.SetOutput(new(plumbing.LogWriter))
		log.SetFlags(0)

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
	app.NewSyslogBindingCache(cfg, m, logger).Run()
}
