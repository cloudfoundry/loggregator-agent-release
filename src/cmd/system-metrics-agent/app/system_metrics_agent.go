package app

import (
	"code.cloudfoundry.org/loggregator-agent/pkg/collector"
	"code.cloudfoundry.org/loggregator-agent/pkg/egress/stats"
	"code.cloudfoundry.org/loggregator-agent/pkg/plumbing"
	"context"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"sync"
	"time"
)

const statOrigin = "system_metrics_agent"

type SystemMetricsAgent struct {
	cfg           Config
	log           *log.Logger
	debugLis      net.Listener
	metricsLis    net.Listener
	metricsServer http.Server
	mu            sync.Mutex
	inputFunc     collector.InputFunc
}

func NewSystemMetricsAgent(i collector.InputFunc, cfg Config, log *log.Logger) *SystemMetricsAgent {
	return &SystemMetricsAgent{
		cfg:       cfg,
		log:       log,
		inputFunc: i,
	}
}

func (a *SystemMetricsAgent) Run() {
	a.startDebugServer()

	metricsURL := fmt.Sprintf(":%d", a.cfg.MetricPort)
	a.startMetricsServer(metricsURL)
}

func (a *SystemMetricsAgent) MetricsAddr() string {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.metricsLis == nil {
		return ""
	}

	return a.metricsLis.Addr().String()
}

func (a *SystemMetricsAgent) DebugAddr() string {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.debugLis == nil {
		return ""
	}

	return a.debugLis.Addr().String()
}

func (a *SystemMetricsAgent) Shutdown(ctx context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.debugLis != nil {
		a.debugLis.Close()
	}

	a.metricsServer.Shutdown(ctx)
}

func (a *SystemMetricsAgent) startDebugServer() {
	a.mu.Lock()
	defer a.mu.Unlock()

	var err error
	a.debugLis, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", a.cfg.DebugPort))
	if err != nil {
		a.log.Panicf("failed to start debug listener: %s", err)
	}

	go http.Serve(a.debugLis, nil)
}

func (a *SystemMetricsAgent) startMetricsServer(addr string) {
	labels := map[string]string{
		"source_id":  statOrigin,
		"deployment": a.cfg.Deployment,
		"job":        a.cfg.Job,
		"index":      a.cfg.Index,
		"ip":         a.cfg.IP,
	}

	promRegisterer := prometheus.NewRegistry()
	promRegistry := stats.NewPromRegistry(promRegisterer)
	promSender := stats.NewPromSender(promRegistry, statOrigin, labels)

	router := http.NewServeMux()
	router.Handle("/metrics", promhttp.HandlerFor(promRegisterer, promhttp.HandlerOpts{}))

	a.setup(addr, router)

	go collector.NewProcessor(
		a.inputFunc,
		[]collector.StatsSender{promSender},
		a.cfg.SampleInterval,
		a.log,
	).Run()

	log.Printf("Metrics server closing: %s", a.metricsServer.ServeTLS(a.metricsLis, "", ""))
}

func (a *SystemMetricsAgent) setup(addr string, router *http.ServeMux) {
	a.mu.Lock()
	defer a.mu.Unlock()

	tlsConfig, err := plumbing.NewServerMutualTLSConfig(
		a.cfg.CertPath,
		a.cfg.KeyPath,
		a.cfg.CACertPath,
	)
	if err != nil {
		log.Fatalf("Unable to setup tls for metrics endpoint (%s): %s", addr, err)
	}

	a.metricsServer = http.Server{
		Addr:         addr,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		Handler:      router,
		TLSConfig:    tlsConfig,
	}

	a.metricsLis, err = net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("Unable to setup metrics endpoint (%s): %s", addr, err)
	}
	log.Printf("Metrics endpoint is listening on %s", a.metricsLis.Addr().String())
}
