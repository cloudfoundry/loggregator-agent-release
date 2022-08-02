package app

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof" //nolint:gosec
	"os"
	"time"

	gendiodes "code.cloudfoundry.org/go-diodes"
	"code.cloudfoundry.org/go-loggregator/v9"
	metrics "code.cloudfoundry.org/go-metric-registry"
	"code.cloudfoundry.org/tlsconfig"

	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/binding"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/cache"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/diodes"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/syslog"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/ingress/bindings"
	v2 "code.cloudfoundry.org/loggregator-agent-release/src/pkg/ingress/v2"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/plumbing"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/timeoutwaitgroup"
	"google.golang.org/grpc"
)

// SyslogAgent manages starting the syslog agent service.
type SyslogAgent struct {
	metrics             Metrics
	pprofPort           uint16
	pprofServer         *http.Server
	debugMetrics        bool
	bindingManager      BindingManager
	grpc                GRPC
	log                 *log.Logger
	bindingsPerAppLimit int
}

type Metrics interface {
	NewGauge(name, helpText string, options ...metrics.MetricOption) metrics.Gauge
	NewCounter(name, helpText string, options ...metrics.MetricOption) metrics.Counter
	RegisterDebugMetrics()
}

type BindingManager interface {
	Run()
	GetDrains(string) []egress.Writer
}

// NewSyslogAgent initializes and returns a new syslog agent.
func NewSyslogAgent(
	cfg Config,
	m Metrics,
	l *log.Logger,
) *SyslogAgent {
	internalTlsConfig, externalTlsConfig := drainTLSConfig(cfg)
	writerFactory := syslog.NewWriterFactory(
		internalTlsConfig,
		externalTlsConfig,
		syslog.NetworkTimeoutConfig{
			Keepalive:    10 * time.Second,
			DialTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		},
		m,
	)

	ingressTLSConfig, err := loggregator.NewIngressTLSConfig(
		cfg.GRPC.CAFile,
		cfg.GRPC.CertFile,
		cfg.GRPC.KeyFile,
	)
	if err != nil {
		l.Panicf("failed to configure client TLS: %q", err)
	}

	logClient, err := loggregator.NewIngressClient(
		ingressTLSConfig,
		loggregator.WithLogger(log.New(os.Stderr, "", log.LstdFlags)),
	)
	if err != nil {
		l.Panicf("failed to create log client for syslog connector: %q", err)
	}

	connector := syslog.NewSyslogConnector(
		cfg.DrainSkipCertVerify,
		timeoutwaitgroup.New(time.Minute),
		writerFactory,
		m,
		syslog.WithLogClient(logClient, "syslog_agent"),
	)

	var cacheClient *cache.CacheClient
	var cupsFetcher binding.Fetcher = nil
	if cfg.Cache.CAFile != "" {
		tlsClient := plumbing.NewTLSHTTPClient(
			cfg.Cache.CertFile,
			cfg.Cache.KeyFile,
			cfg.Cache.CAFile,
			cfg.Cache.CommonName,
		)

		cacheClient = cache.NewClient(cfg.Cache.URL, tlsClient)
		cupsFetcher = bindings.NewFilteredBindingFetcher(
			&cfg.Cache.Blacklist,
			bindings.NewBindingFetcher(cfg.BindingsPerAppLimit, cacheClient, m, cfg.LegacyBehaviour),
			m,
			l,
		)
		cupsFetcher = bindings.NewDrainParamParser(cupsFetcher, cfg.DefaultDrainMetadata)
	}

	aggregateFetcher := bindings.NewAggregateDrainFetcher(cfg.AggregateDrainURLs, cacheClient)
	bindingManager := binding.NewManager(
		cupsFetcher,
		bindings.NewDrainParamParser(aggregateFetcher, cfg.DefaultDrainMetadata),
		connector,
		m,
		cfg.Cache.PollingInterval,
		cfg.IdleDrainTimeout,
		cfg.AggregateConnectionRefreshInterval,
		l,
	)

	return &SyslogAgent{
		grpc:                cfg.GRPC,
		debugMetrics:        cfg.MetricsServer.DebugMetrics,
		pprofPort:           cfg.MetricsServer.PprofPort,
		metrics:             m,
		log:                 l,
		bindingsPerAppLimit: cfg.BindingsPerAppLimit,
		bindingManager:      bindingManager,
	}
}

func drainTLSConfig(cfg Config) (*tls.Config, *tls.Config) {
	certPool := trustedCertPool(cfg.DrainTrustedCAFile)
	internalTlsConfig, err := tlsconfig.Build(
		tlsconfig.WithInternalServiceDefaults(),
	).Client(
		tlsconfig.WithAuthority(certPool),
	)
	if err != nil {
		log.Panicf("failed to load create tls config for http client: %s", err)
	}

	externalTlsConfig, err := tlsconfig.Build(
		tlsconfig.WithExternalServiceDefaults(),
	).Client(
		tlsconfig.WithAuthority(certPool),
	)

	if err != nil {
		log.Panicf("failed to load create tls config for http client: %s", err)
	}
	cipherSuites, err := cfg.processCipherSuites()
	if err != nil {
		log.Panicf("failed to load create tls config for http client: %s", err)
	}
	if cipherSuites != nil {
		externalTlsConfig, err = tlsconfig.Build(
			func(c *tls.Config) error {
				c.MinVersion = tls.VersionTLS12
				c.MaxVersion = tls.VersionTLS12
				c.CipherSuites = *cipherSuites
				return nil
			},
		).Client(
			tlsconfig.WithAuthority(certPool),
		)
		if err != nil {
			log.Panicf("failed to load create tls config for http client: %s", err)
		}

	}

	internalTlsConfig.InsecureSkipVerify = cfg.DrainSkipCertVerify //nolint:gosec
	externalTlsConfig.InsecureSkipVerify = cfg.DrainSkipCertVerify //nolint:gosec

	return internalTlsConfig, externalTlsConfig
}

func trustedCertPool(trustedCAFile string) *x509.CertPool {
	cp, err := x509.SystemCertPool()
	if err != nil {
		cp = x509.NewCertPool()
	}

	if trustedCAFile != "" {
		cert, err := os.ReadFile(trustedCAFile)
		if err != nil {
			log.Printf("unable to read provided custom CA: %s", err)
			return cp
		}

		ok := cp.AppendCertsFromPEM(cert)
		if !ok {
			log.Println("unable to add provided custom CA")
		}
	}

	return cp
}

func (s *SyslogAgent) Run() {
	if s.debugMetrics {
		s.metrics.RegisterDebugMetrics()
		s.pprofServer = &http.Server{
			Addr:              fmt.Sprintf("127.0.0.1:%d", s.pprofPort),
			Handler:           http.DefaultServeMux,
			ReadHeaderTimeout: 2 * time.Second,
		}
		go func() { log.Println("PPROF SERVER STOPPED " + s.pprofServer.ListenAndServe().Error()) }()
	}
	ingressDropped := s.metrics.NewCounter(
		"dropped",
		"Total number of dropped envelopes.",
		metrics.WithMetricLabels(map[string]string{"direction": "ingress"}),
	)
	diode := diodes.NewManyToOneEnvelopeV2(10000, gendiodes.AlertFunc(func(missed int) {
		ingressDropped.Add(float64(missed))
	}))
	go s.bindingManager.Run()

	drainIngress := s.metrics.NewCounter(
		"ingress",
		"Total number of envelopes ingressed by the agent.",
		metrics.WithMetricLabels(map[string]string{"scope": "all_drains"}),
	)
	envelopeWriter := syslog.NewEnvelopeWriter(s.bindingManager.GetDrains, diode.Next, drainIngress, s.log)
	go envelopeWriter.Run()

	var opts []plumbing.ConfigOption
	if len(s.grpc.CipherSuites) > 0 {
		opts = append(opts, plumbing.WithCipherSuites(s.grpc.CipherSuites))
	}

	serverCreds, err := plumbing.NewServerCredentials(
		s.grpc.CertFile,
		s.grpc.KeyFile,
		s.grpc.CAFile,
		opts...,
	)
	if err != nil {
		s.log.Fatalf("failed to configure server TLS: %s", err)
	}

	im := s.metrics.NewCounter(
		"ingress",
		"Total number of envelopes ingressed by the agent.",
		metrics.WithMetricLabels(map[string]string{"scope": "agent"}),
	)
	omm := s.metrics.NewCounter(
		"origin_mappings",
		"Total number of envelopes where the origin tag is used as the source_id.",
	)

	rx := v2.NewReceiver(diode, im, omm)
	srv := v2.NewServer(
		fmt.Sprintf("127.0.0.1:%d", s.grpc.Port),
		rx,
		grpc.Creds(serverCreds),
		grpc.MaxRecvMsgSize(10*1024*1024),
	)
	srv.Start()
}

func (s *SyslogAgent) Stop() {
	if s.pprofServer != nil {
		s.pprofServer.Close()
	}
}
