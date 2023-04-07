package app

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof" //nolint:gosec
	"sync"
	"time"

	metrics "code.cloudfoundry.org/go-metric-registry"
	"code.cloudfoundry.org/tlsconfig"

	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/binding"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/cache"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/ingress/api"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/plumbing"
	"github.com/go-chi/chi/v5"
)

type SyslogBindingCache struct {
	config      Config
	pprofServer *http.Server
	server      *http.Server
	log         *log.Logger
	metrics     Metrics
	mu          sync.Mutex
}

type Metrics interface {
	NewCounter(name, helpText string, options ...metrics.MetricOption) metrics.Counter
	NewGauge(name, helpText string, o ...metrics.MetricOption) metrics.Gauge
	RegisterDebugMetrics()
}

func NewSyslogBindingCache(config Config, metrics Metrics, log *log.Logger) *SyslogBindingCache {
	return &SyslogBindingCache{
		config:  config,
		log:     log,
		metrics: metrics,
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
	legacyStore := binding.NewLegacyStore()
	aggregateStore := binding.NewAggregateStore(sbc.config.AggregateDrainsFile)
	poller := binding.NewPoller(sbc.apiClient(), sbc.config.APIPollingInterval, store, legacyStore, sbc.metrics, sbc.log)

	go poller.Poll()

	router := chi.NewRouter()
	router.HandleFunc("/bindings", cache.LegacyHandler(legacyStore))
	router.HandleFunc("/v2/bindings", cache.Handler(store))
	router.HandleFunc("/aggregate", cache.LegacyAggregateHandler(aggregateStore))
	router.HandleFunc("/v2/aggregate", cache.AggregateHandler(aggregateStore))

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
