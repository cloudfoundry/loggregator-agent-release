package app

import (
	"fmt"
	"log"
	"net"
	"net/http"

	"code.cloudfoundry.org/loggregator-agent/pkg/binding"
	"code.cloudfoundry.org/loggregator-agent/pkg/cache"
	"code.cloudfoundry.org/loggregator-agent/pkg/ingress/api"
	"code.cloudfoundry.org/loggregator-agent/pkg/plumbing"
	"github.com/gorilla/mux"
)

type SyslogBindingCache struct {
	config Config
	log    *log.Logger
}

func NewSyslogBindingCache(config Config, log *log.Logger) *SyslogBindingCache {
	return &SyslogBindingCache{
		config: config,
		log:    log,
	}
}

func (sbc *SyslogBindingCache) Run() {
	listenAddr := fmt.Sprintf(":%d", sbc.config.CachePort)
	lis, err := net.Listen("tcp", listenAddr)
	if err != nil {
		sbc.log.Panicf("error creating listener: %s", err)
	}

	store := binding.NewStore()
	poller := binding.NewPoller(sbc.apiClient(), sbc.config.APIPollingInterval, store)

	go poller.Poll()

	router := mux.NewRouter()
	router.HandleFunc("/bindings", cache.Handler(store)).Methods(http.MethodGet)

	var opts []plumbing.ConfigOption
	if len(sbc.config.CipherSuites) > 0 {
		opts = append(opts, plumbing.WithCipherSuites(sbc.config.CipherSuites))
	}

	tlsConfig, err := plumbing.NewServerMutualTLSConfig(
		sbc.config.CacheCertFile,
		sbc.config.CacheKeyFile,
		sbc.config.CacheCAFile,
		opts...,
	)
	if err != nil {
		sbc.log.Panicf("failed to load server TLS config: %s", err)
	}

	server := &http.Server{
		Handler:   router,
		TLSConfig: tlsConfig,
	}

	server.ServeTLS(lis, "", "")
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
