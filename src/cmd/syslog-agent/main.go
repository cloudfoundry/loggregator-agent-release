package main

import (
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/syslog"
	"log"
	_ "net/http/pprof" //nolint:gosec
	"os"

	metrics "code.cloudfoundry.org/go-metric-registry"

	"code.cloudfoundry.org/loggregator-agent-release/src/cmd/syslog-agent/app"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/plumbing"
)

func main() {
	logger := log.New(os.Stderr, "", log.LstdFlags)
	logger.Println("starting syslog-agent")
	defer logger.Println("stopping syslog-agent")

	cfg := app.LoadConfig()
	if cfg.UseRFC3339 {
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

	factory := syslog.NewAppLogEmitterFactory()

	app.NewSyslogAgent(cfg, m, logger, &factory).Run()
}
