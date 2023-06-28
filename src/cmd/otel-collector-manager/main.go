package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"

	"code.cloudfoundry.org/loggregator-agent-release/src/cmd/otel-collector-manager/app"
	"code.cloudfoundry.org/loggregator-agent-release/src/cmd/otel-collector-manager/app/collector"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/cache"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/plumbing"
)

func main() {
	l := logrus.New()
	l.Out = os.Stdout

	config, err := app.LoadConfig()
	if err != nil {
		log.Fatalf("Unable to parse config: %s", err)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	cw := collector.NewConfigWriter(
		config.CollectorBaseConfig,
		config.CollectorRunningConfig,
	)

	stdoutLog, err := os.OpenFile(config.CollectorStdoutLog, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		panic(err)
	}

	stderrLog, err := os.OpenFile(config.CollectorStderrLog, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		panic(err)
	}

	r := collector.NewRunner(
		config.CollectorPidFile,
		config.CollectorBinary,
		[]string{"--config", config.CollectorRunningConfig},
		stdoutLog,
		stderrLog,
		time.Second,
		l,
	)
	a := collector.NewConfigApplier(config.CollectorPidFile)

	var cc *cache.CacheClient
	tlsClient := plumbing.NewTLSHTTPClient(
		config.Cache.CertFile,
		config.Cache.KeyFile,
		config.Cache.CAFile,
		config.Cache.CommonName,
	)
	cc = cache.NewClient(config.Cache.URL, tlsClient)

	g := collector.NewChangeGetter(cc)
	m := app.NewManager(g, 30*time.Second, cw, r, a, l)

	ctx, cancel := context.WithCancel(context.Background())
	stoppedCh := make(chan struct{}, 1)
	go m.Run(ctx, stoppedCh)

	select {
	case sig := <-sigCh:
		cancel()
		l.WithField("signal", sig).Info("received signal")
	}
	<-stoppedCh

	l.Info("OTel Manager Stopped")
}
