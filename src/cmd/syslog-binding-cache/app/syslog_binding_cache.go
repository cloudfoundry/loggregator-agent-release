package app

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"

	metrics "code.cloudfoundry.org/go-metric-registry"
	"code.cloudfoundry.org/tlsconfig"

	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/binding"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/cache"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/ingress/api"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/plumbing"
	"github.com/gorilla/mux"
)

type SyslogBindingCache struct {
	config  Config
	log     *log.Logger
	metrics Metrics
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
		go http.ListenAndServe(fmt.Sprintf("127.0.0.1:%d", sbc.config.MetricsServer.PprofPort), nil)
	}
	store := binding.NewStore(sbc.metrics)
	aggregateStore := binding.AggregateStore{AggregateDrains: sbc.config.AggregateDrains}
	poller := binding.NewPoller(sbc.apiClient(), sbc.config.APIPollingInterval, store, sbc.metrics, sbc.log)

	go poller.Poll()

	router := mux.NewRouter()
	router.HandleFunc("/bindings", cache.Handler(store)).Methods(http.MethodGet)
	router.HandleFunc("/aggregate", cache.Handler(&aggregateStore)).Methods(http.MethodGet)

	sbc.startServer(router)
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

func (sbc *SyslogBindingCache) startServer(router *mux.Router) {
	listenAddr := fmt.Sprintf(":%d", sbc.config.CachePort)
	lis, err := net.Listen("tcp", listenAddr)
	if err != nil {
		sbc.log.Panicf("error creating listener: %s", err)
	}

	server := &http.Server{
		Handler:   router,
		TLSConfig: sbc.tlsConfig(),
	}
	server.ServeTLS(lis, "", "")
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
