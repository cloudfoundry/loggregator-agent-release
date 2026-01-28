package app

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof" //nolint:gosec
	"os"
	"sync"
	"time"

	"code.cloudfoundry.org/go-loggregator/v10"
	metrics "code.cloudfoundry.org/go-metric-registry"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/applog"
	"code.cloudfoundry.org/tlsconfig"

	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/binding"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/cache"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/ingress/api"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/plumbing"
	"github.com/go-chi/chi/v5"
)

type IPChecker interface {
	ResolveAddr(host string) (net.IP, error)
	CheckBlacklist(ip net.IP) error
}

type SyslogBindingCache struct {
	config      Config
	pprofServer *http.Server
	server      *http.Server
	log         *log.Logger
	metrics     Metrics
	mu          sync.Mutex
	emitter     applog.LogEmitter
	checker     IPChecker
}

type Metrics interface {
	NewCounter(name, helpText string, options ...metrics.MetricOption) metrics.Counter
	NewGauge(name, helpText string, o ...metrics.MetricOption) metrics.Gauge
	RegisterDebugMetrics()
}

func NewSyslogBindingCache(config Config, metrics Metrics, logger *log.Logger) *SyslogBindingCache {
	ingressTLSConfig, err := loggregator.NewIngressTLSConfig(
		config.GRPC.CAFile,
		config.GRPC.CertFile,
		config.GRPC.KeyFile,
	)
	if err != nil {
		logger.Panicf("failed to configure client TLS: %q", err)
	}

	logClient, err := loggregator.NewIngressClient(
		ingressTLSConfig,
		loggregator.WithLogger(log.New(os.Stderr, "", log.LstdFlags)),
		loggregator.WithAddr(config.ForwarderAgentAddress),
	)
	if err != nil {
		logger.Panicf("failed to create logger client for syslog binding cache: %q", err)
	}
	factory := applog.NewAppLogEmitterFactory()
	emitter := factory.NewLogEmitter(logClient, "syslog_binding_cache")

	return &SyslogBindingCache{
		config:  config,
		log:     logger,
		metrics: metrics,
		emitter: emitter,
		checker: &config.Blacklist,
	}
}

func (sbc *SyslogBindingCache) Run() {
	if sbc.config.MetricsServer.DebugMetrics {
		sbc.metrics.RegisterDebugMetrics()
		sbc.pprofServer = &http.Server{
			Addr:              fmt.Sprintf("127.0.0.1:%d", sbc.config.MetricsServer.PprofPort),
			Handler:           http.DefaultServeMux,
			ReadHeaderTimeout: 2 * time.Second,
		}
		go func() { sbc.log.Println("PPROF SERVER STOPPED " + sbc.pprofServer.ListenAndServe().Error()) }()
	}
	store := binding.NewStore(sbc.metrics)
	aggregateStore := binding.NewAggregateStore(sbc.config.AggregateDrainsFile)
	poller := binding.NewPoller(sbc.apiClient(), sbc.config.APIPollingInterval, store, sbc.metrics, sbc.log, sbc.emitter, &sbc.config.Blacklist)

	go poller.Poll()

	router := chi.NewRouter()
	router.Get("/v2/bindings", cache.Handler(store))
	router.Get("/v2/aggregate", cache.AggregateHandler(aggregateStore))

	sbc.startServer(router)
}

func (sbc *SyslogBindingCache) Stop() {
	if sbc.pprofServer != nil {
		sbc.pprofServer.Close()
	}
	sbc.mu.Lock()
	defer sbc.mu.Unlock()
	if sbc.server != nil {
		sbc.server.Close()
	}
}
func (sbc *SyslogBindingCache) apiClient() api.Client {
	httpClient := plumbing.NewTLSHTTPClient(
		sbc.config.APICertFile,
		sbc.config.APIKeyFile,
		sbc.config.APICAFile,
		sbc.config.APICommonName,
		sbc.config.APIDisableKeepAlives,
	)

	return api.Client{
		Addr:      sbc.config.APIURL,
		Client:    httpClient,
		BatchSize: sbc.config.APIBatchSize,
	}
}

func (sbc *SyslogBindingCache) startServer(router chi.Router) {
	listenAddr := fmt.Sprintf(":%d", sbc.config.CachePort)
	sbc.mu.Lock()
	sbc.server = &http.Server{
		Addr:              listenAddr,
		Handler:           router,
		TLSConfig:         sbc.tlsConfig(),
		ReadHeaderTimeout: 2 * time.Second,
	}
	sbc.mu.Unlock()
	err := sbc.server.ListenAndServeTLS("", "")
	if err != http.ErrServerClosed {
		sbc.log.Panicf("error creating listener: %s", err)
	}
}

func (sbc *SyslogBindingCache) tlsConfig() *tls.Config {
	tlsConfig, err := tlsconfig.Build(
		tlsconfig.WithInternalServiceDefaults(),
		tlsconfig.WithIdentityFromFile(sbc.config.CacheCertFile, sbc.config.CacheKeyFile),
	).Server(
		tlsconfig.WithClientAuthenticationFromFile(sbc.config.CacheCAFile),
	)
	if err != nil {
		sbc.log.Panicf("failed to load server TLS config: %s", err)
	}

	if len(sbc.config.CipherSuites) > 0 {
		opt := plumbing.WithCipherSuites(sbc.config.CipherSuites)
		opt(tlsConfig)
	}

	return tlsConfig
}
