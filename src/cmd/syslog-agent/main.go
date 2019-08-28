package main

import (
	"code.cloudfoundry.org/go-loggregator/metrics"
	"log"
	_ "net/http/pprof"
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
		metrics.WithTLSServer(
			int(cfg.MetricsServer.Port),
			cfg.MetricsServer.CertFile,
			cfg.MetricsServer.KeyFile,
			cfg.MetricsServer.CAFile,
		),
	)

	app.NewSyslogAgent(cfg, m, log).Run()
}
