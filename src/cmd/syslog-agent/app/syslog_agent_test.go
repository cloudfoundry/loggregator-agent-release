package app_test

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/go-loggregator/v9"
	"code.cloudfoundry.org/go-loggregator/v9/rfc5424"
	"code.cloudfoundry.org/go-loggregator/v9/rpc/loggregator_v2"
	metricsHelpers "code.cloudfoundry.org/go-metric-registry/testhelpers"
	"code.cloudfoundry.org/loggregator-agent-release/src/cmd/syslog-agent/app"
	"code.cloudfoundry.org/loggregator-agent-release/src/internal/testhelper"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/binding"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/config"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/ingress/bindings"
	"code.cloudfoundry.org/tlsconfig"
)

var _ = Describe("SyslogAgent", func() {
	const sleepTime = 2 * time.Second

	var (
		grpcPort    int
		pprofPort   int
		metricsPort int

		appHTTPSDrain  *syslogHTTPSServer
		appTLSDrain    *syslogTCPServer
		aggregateDrain *syslogTCPServer
		appIDs         []string
		cacheCerts     *testhelper.TestCerts
		bindingCache   *fakeLegacyBindingCache

		agentCerts   *testhelper.TestCerts
		agentCfg     app.Config
		agentMetrics *metricsHelpers.SpyMetricsRegistry
		agentLogr    *log.Logger
		agent        *app.SyslogAgent
	)

	BeforeEach(func() {
		grpcPort = 30000 + GinkgoParallelProcess()
		pprofPort = 31000 + GinkgoParallelProcess()
		metricsPort = 32000 + GinkgoParallelProcess()

		drainCerts := testhelper.GenerateCerts("drain-ca")
		appHTTPSDrain = newSyslogHTTPSServer(drainCerts, "")
		appTLSDrain = newSyslogTLSServer(drainCerts, tlsconfig.WithInternalServiceDefaults(), "")
		aggregateDrain = newSyslogTLSServer(drainCerts, tlsconfig.WithInternalServiceDefaults(), "")

		appIDs = []string{"app-1", "app-2"}
		cacheCerts = testhelper.GenerateCerts("binding-cache-ca")
		bindingCache = &fakeLegacyBindingCache{
			bindings: []binding.LegacyBinding{
				{
					AppID:    appIDs[0],
					Hostname: fmt.Sprintf("%s.example.com", appIDs[0]),
					Drains:   []string{appHTTPSDrain.server.URL},
				},
				{
					AppID:    appIDs[1],
					Hostname: fmt.Sprintf("%s.example.com", appIDs[1]),
					Drains: []string{
						fmt.Sprintf("syslog-tls://localhost:%s", appTLSDrain.port()),
					},
				},
			},
			aggregate: []binding.LegacyBinding{
				{
					Drains: []string{
						fmt.Sprintf("syslog-tls://localhost:%s", aggregateDrain.port()),
					},
				},
			},
		}

		agentCerts = testhelper.GenerateCerts("metron-ca")
		agentCfg = app.Config{
			AggregateConnectionRefreshInterval: 1 * time.Minute,
			BindingsPerAppLimit:                5,
			DefaultDrainMetadata:               true,
			DrainTrustedCAFile:                 drainCerts.CA(),
			GRPC: app.GRPC{
				Port:     grpcPort,
				CAFile:   agentCerts.CA(),
				CertFile: agentCerts.Cert("metron"),
				KeyFile:  agentCerts.Key("metron"),
			},
			IdleDrainTimeout: 10 * time.Minute,
			MetricsServer: config.MetricsServer{
				Port:      uint16(metricsPort),
				CAFile:    agentCerts.CA(),
				CertFile:  agentCerts.Cert("metron"),
				KeyFile:   agentCerts.Key("metron"),
				PprofPort: uint16(pprofPort),
			},
		}
		agentMetrics = metricsHelpers.NewMetricsRegistry()
		agentLogr = log.New(GinkgoWriter, "", log.LstdFlags)
	})

	JustBeforeEach(func() {
		if bindingCache != nil {
			bindingCache.startTLS(cacheCerts)
			agentCfg.Cache.URL = bindingCache.URL
			agentCfg.Cache.CAFile = cacheCerts.CA()
			agentCfg.Cache.CertFile = cacheCerts.Cert("binding-cache")
			agentCfg.Cache.KeyFile = cacheCerts.Key("binding-cache")
			agentCfg.Cache.CommonName = "binding-cache"
			agentCfg.Cache.PollingInterval = 10 * time.Millisecond
		}

		agent = app.NewSyslogAgent(agentCfg, agentMetrics, agentLogr)
		go agent.Run()
	})

	AfterEach(func() {
		agent.Stop()
		if bindingCache != nil {
			bindingCache.Close()
		}
		aggregateDrain.lis.Close()
		appTLSDrain.lis.Close()
		appHTTPSDrain.server.Close()
	})

	It("connects to drains", func() {
		ctx, cancel := context.WithCancel(context.Background())
		emitLogs(ctx, appIDs, grpcPort, agentCerts)
		defer cancel()

		Eventually(func() float64 {
			return agentMetrics.GetMetric("active_drains", map[string]string{"unit": "count"}).Value()
		}, 3).Should(Equal(3.0))

		var msg *rfc5424.Message

		Eventually(func() float64 {
			return agentMetrics.GetMetric("aggregate_drains", map[string]string{"unit": "count"}).Value()
		}, 3).Should(Equal(1.0))
		Eventually(aggregateDrain.receivedMessages, 3).Should(Receive(&msg))
		Expect(msg.StructuredData).NotTo(HaveLen(0))
		Expect(msg.StructuredData[0].ID).To(Equal("tags@47450"))

		Eventually(func() float64 {
			return agentMetrics.GetMetric("drains", map[string]string{"unit": "count"}).Value()
		}, 3).Should(Equal(2.0))
		Eventually(appHTTPSDrain.receivedMessages, 3).Should(Receive(&msg))
		Expect(msg.StructuredData).NotTo(HaveLen(0))
		Expect(msg.StructuredData[0].ID).To(Equal("tags@47450"))
		Eventually(appTLSDrain.receivedMessages, 3).Should(Receive(&msg))
		Expect(msg.StructuredData).NotTo(HaveLen(0))
		Expect(msg.StructuredData[0].ID).To(Equal("tags@47450"))
	})

	It("generates metrics", func() {
		// Give agent.Run() time to start the gRPC server, otherwise the
		// following assertions complete too fast and agent.Stop() errors.
		time.Sleep(sleepTime)

		metrics := []struct {
			name   string
			labels map[string]string
		}{
			{
				name:   "dropped",
				labels: map[string]string{"direction": "ingress"},
			},
			{
				name:   "ingress",
				labels: map[string]string{"scope": "agent"},
			},
			{
				name:   "drains",
				labels: map[string]string{"unit": "count"},
			},
			{
				name:   "aggregate_drains",
				labels: map[string]string{"unit": "count"},
			},
			{
				name:   "active_drains",
				labels: map[string]string{"unit": "count"},
			},
			{
				name:   "binding_refresh_count",
				labels: nil,
			},
			{
				name:   "latency_for_last_binding_refresh",
				labels: map[string]string{"unit": "ms"},
			},
			{
				name:   "ingress",
				labels: map[string]string{"scope": "all_drains"},
			},
			{
				name:   "dropped",
				labels: map[string]string{"direction": "egress"},
			},
			{
				name: "egress",
				labels: map[string]string{
					"direction":   "egress",
					"drain_scope": "aggregate",
					"drain_url":   bindingCache.aggregate[0].Drains[0],
				},
			},
		}
		for _, m := range metrics {
			Eventually(agentMetrics.HasMetric).WithArguments(m.name, m.labels).Should(BeTrue())
		}

		Eventually(agentMetrics.GetDebugMetricsEnabled).Should(BeFalse())
	})

	Context("when debug configuration is enabled", func() {
		BeforeEach(func() {
			agentCfg.MetricsServer.DebugMetrics = true
		})

		It("registers a pprof endpoint", func() {
			// Give agent.Run() time to start the gRPC server, otherwise the
			// following assertions complete too fast and agent.Stop() errors.
			time.Sleep(sleepTime)

			u := fmt.Sprintf("http://127.0.0.1:%d/debug/pprof/", agentCfg.MetricsServer.PprofPort)
			Eventually(func() bool {
				resp, err := http.Get(u) //nolint:gosec
				if err != nil {
					return false
				}
				defer resp.Body.Close()
				return resp.StatusCode == 200
			}, 3).Should(BeTrue())
		})

		It("registers debug metrics", func() {
			// Give agent.Run() time to start the gRPC server, otherwise the
			// following assertions complete too fast and agent.Stop() errors.
			time.Sleep(sleepTime)

			Eventually(agentMetrics.GetDebugMetricsEnabled).Should(BeTrue())
		})
	})

	Context("when IPs are added to the denylist configuration", func() {
		BeforeEach(func() {
			url, err := url.Parse(appHTTPSDrain.server.URL)
			Expect(err).NotTo(HaveOccurred())
			agentCfg.Cache.Blacklist = bindings.BlacklistRanges{
				Ranges: []bindings.BlacklistRange{
					{
						Start: url.Hostname(),
						End:   url.Hostname(),
					},
				},
			}
		})

		It("does not send logs to those IPs", func() {
			ctx, cancel := context.WithCancel(context.Background())
			emitLogs(ctx, appIDs, grpcPort, agentCerts)
			defer cancel()

			Consistently(appHTTPSDrain.receivedMessages, 7).ShouldNot(Receive())
		})
	})

	Context("when default drain meta data configuration is false", func() {
		BeforeEach(func() {
			agentCfg.DefaultDrainMetadata = false

			oldURL := bindingCache.aggregate[0].Drains[0]
			bindingCache.aggregate[0].Drains[0] = fmt.Sprintf("%s?disable-metadata=false", oldURL)
		})

		It("does not include tags in drains that do not set disable-metadata to false", func() {
			ctx, cancel := context.WithCancel(context.Background())
			emitLogs(ctx, appIDs, grpcPort, agentCerts)
			defer cancel()

			var msg *rfc5424.Message

			Eventually(aggregateDrain.receivedMessages, 3).Should(Receive(&msg))
			Expect(msg.StructuredData).NotTo(HaveLen(0))
			Expect(msg.StructuredData[0].ID).To(Equal("tags@47450"))

			Eventually(appHTTPSDrain.receivedMessages, 3).Should(Receive(&msg))
			Expect(msg.StructuredData).To(HaveLen(0))

			Eventually(appTLSDrain.receivedMessages, 3).Should(Receive(&msg))
			Expect(msg.StructuredData).To(HaveLen(0))
		})
	})

	Context("when the disable-metadata param is set in drain URLS", func() {
		BeforeEach(func() {
			agentCfg.DefaultDrainMetadata = true

			oldURL := bindingCache.aggregate[0].Drains[0]
			bindingCache.aggregate[0].Drains[0] = fmt.Sprintf("%s?disable-metadata=true", oldURL)
			oldURL = bindingCache.bindings[0].Drains[0]
			bindingCache.bindings[0].Drains[0] = fmt.Sprintf("%s?disable-metadata=true", oldURL)
		})

		It("does not send tags to those drains", func() {
			ctx, cancel := context.WithCancel(context.Background())
			emitLogs(ctx, appIDs, grpcPort, agentCerts)
			defer cancel()

			var msg *rfc5424.Message

			Eventually(aggregateDrain.receivedMessages, 3).Should(Receive(&msg))
			Expect(msg.StructuredData).To(HaveLen(0))

			Eventually(appHTTPSDrain.receivedMessages, 3).Should(Receive(&msg))
			Expect(msg.StructuredData).To(HaveLen(0))

			Eventually(appTLSDrain.receivedMessages, 3).Should(Receive(&msg))
			Expect(msg.StructuredData).NotTo(HaveLen(0))
			Expect(msg.StructuredData[0].ID).To(Equal("tags@47450"))
		})
	})

	Context("when the drain cipher suites are not compatible with a drain", func() {
		BeforeEach(func() {
			// Could be any cipher that does not match those in
			// tlsconfig.WithInternalServiceDefaults().
			agentCfg.DrainCipherSuites = "TLS_RSA_WITH_3DES_EDE_CBC_SHA"
		})

		It("refuses to connect to that drain", func() {
			ctx, cancel := context.WithCancel(context.Background())
			emitLogs(ctx, appIDs, grpcPort, agentCerts)
			defer cancel()

			Consistently(appTLSDrain.receivedMessages, 10).ShouldNot(Receive())
		})

		Context("when the ssl-strict-internal param is set in that drain URL", func() {
			BeforeEach(func() {
				oldURL := bindingCache.aggregate[0].Drains[0]
				bindingCache.aggregate[0].Drains[0] = fmt.Sprintf("%s?ssl-strict-internal=true", oldURL)
			})

			It("uses internal TLS settings to communicate with that drain", func() {
				ctx, cancel := context.WithCancel(context.Background())
				emitLogs(ctx, appIDs, grpcPort, agentCerts)
				defer cancel()

				Eventually(aggregateDrain.receivedMessages, 3).Should(Receive())
			})
		})
	})

	Context("when binding cache configuration is empty", func() {
		BeforeEach(func() {
			agentCfg.AggregateDrainURLs = []string{fmt.Sprintf("syslog-tls://localhost:%s", aggregateDrain.port())}

			bindingCache = nil
		})

		It("only connects to the aggregate drains in its own configuration", func() {
			ctx, cancel := context.WithCancel(context.Background())
			emitLogs(ctx, appIDs, grpcPort, agentCerts)
			defer cancel()

			Eventually(func() float64 {
				return agentMetrics.GetMetric("active_drains", map[string]string{"unit": "count"}).Value()
			}, 3).Should(Equal(1.0))

			Eventually(func() float64 {
				return agentMetrics.GetMetric("aggregate_drains", map[string]string{"unit": "count"}).Value()
			}, 3).Should(Equal(1.0))

			Eventually(aggregateDrain.receivedMessages, 3).Should(Receive())
		})

		It("does not connect to app drains", func() {
			Consistently(func() float64 {
				return agentMetrics.GetMetric("drains", map[string]string{"unit": "count"}).Value()
			}, 3).Should(Equal(0.0))

			Consistently(appHTTPSDrain.receivedMessages, 5).ShouldNot(Receive())
			Consistently(appTLSDrain.receivedMessages, 5).ShouldNot(Receive())
		})
	})

	Context("when GRPC cert configuration is invalid", func() {
		It("panics", func() {
			// Give agent.Run() time to start the gRPC server, otherwise the
			// following assertions complete too fast and agent.Stop() errors.
			time.Sleep(sleepTime)

			cfgCopy := agentCfg
			cfgCopy.GRPC.CAFile = "invalid"
			cfgCopy.GRPC.CertFile = "invalid"
			cfgCopy.GRPC.KeyFile = "invalid"

			msg := `failed to configure client TLS: "failed to load keypair: open invalid: no such file or directory"`
			Expect(func() { app.NewSyslogAgent(cfgCopy, agentMetrics, agentLogr) }).To(PanicWith(msg))
		})
	})
})

func emitLogs(ctx context.Context, appIDs []string, grpcPort int, testCerts *testhelper.TestCerts) {
	tlsConfig, err := loggregator.NewIngressTLSConfig(
		testCerts.CA(),
		testCerts.Cert("metron"),
		testCerts.Key("metron"),
	)
	Expect(err).ToNot(HaveOccurred())
	ingressClient, err := loggregator.NewIngressClient(
		tlsConfig,
		loggregator.WithAddr(fmt.Sprintf("127.0.0.1:%d", grpcPort)),
		loggregator.WithLogger(log.New(GinkgoWriter, "[TEST INGRESS CLIENT] ", 0)),
		loggregator.WithBatchMaxSize(1),
	)
	Expect(err).ToNot(HaveOccurred())

	var envelopes []*loggregator_v2.Envelope
	for _, id := range appIDs {
		e := &loggregator_v2.Envelope{
			Timestamp: time.Now().UnixNano(),
			SourceId:  id,
			Message: &loggregator_v2.Envelope_Log{
				Log: &loggregator_v2.Log{
					Payload: []byte("hello"),
				},
			},
			Tags: map[string]string{
				"foo": "bar",
			},
		}
		envelopes = append(envelopes, e)
	}

	go func() {
		ticker := time.NewTicker(10 * time.Millisecond)
		for {
			select {
			case <-ticker.C:
				for _, e := range envelopes {
					ingressClient.Emit(e)
				}
			case <-ctx.Done():
				ticker.Stop()
				return
			}
		}
	}()
}

type fakeLegacyBindingCache struct {
	*httptest.Server
	bindings  []binding.LegacyBinding
	aggregate []binding.LegacyBinding
}

func (f *fakeLegacyBindingCache) startTLS(testCerts *testhelper.TestCerts) {
	tlsConfig, err := tlsconfig.Build(
		tlsconfig.WithInternalServiceDefaults(),
		tlsconfig.WithIdentityFromFile(
			testCerts.Cert("binding-cache"),
			testCerts.Key("binding-cache"),
		),
	).Server(
		tlsconfig.WithClientAuthenticationFromFile(testCerts.CA()),
	)

	Expect(err).ToNot(HaveOccurred())

	f.Server = httptest.NewUnstartedServer(f)
	f.Server.TLS = tlsConfig
	f.Server.StartTLS()
}

func (f *fakeLegacyBindingCache) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var results []binding.LegacyBinding
	if r.URL.Path == "/bindings" {
		results = f.bindings
	} else if r.URL.Path == "/aggregate" {
		results = f.aggregate
	} else {
		w.WriteHeader(500)
		return
	}

	resultData, err := json.Marshal(results)
	if err != nil {
		w.WriteHeader(500)
		return
	}

	_, err = w.Write(resultData)
	if err != nil {
		w.WriteHeader(500)
	}
}

type syslogHTTPSServer struct {
	receivedMessages chan *rfc5424.Message
	server           *httptest.Server
}

func newSyslogHTTPSServer(syslogServerTestCerts *testhelper.TestCerts, clientCAFile string) *syslogHTTPSServer {
	syslogServer := syslogHTTPSServer{
		receivedMessages: make(chan *rfc5424.Message, 100),
	}
	serverOptions := []tlsconfig.ServerOption{}
	if clientCAFile != "" {
		serverOptions = append(serverOptions, tlsconfig.WithClientAuthenticationFromFile(clientCAFile))
	}

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		msg := &rfc5424.Message{}

		data, err := io.ReadAll(r.Body)
		defer r.Body.Close()
		if err != nil {
			panic(err)
		}

		err = msg.UnmarshalBinary(data)
		if err != nil {
			panic(err)
		}

		syslogServer.receivedMessages <- msg
	}))

	tlsConfig, err := tlsconfig.Build(
		tlsconfig.WithInternalServiceDefaults(),
		tlsconfig.WithIdentityFromFile(
			syslogServerTestCerts.Cert("localhost"),
			syslogServerTestCerts.Key("localhost"),
		),
	).Server(
		serverOptions...,
	)
	if err != nil {
		panic(err)
	}

	server.TLS = tlsConfig
	server.StartTLS()

	syslogServer.server = server
	return &syslogServer
}

type syslogTCPServer struct {
	lis              net.Listener
	receivedMessages chan *rfc5424.Message
}

func newSyslogTLSServer(syslogServerTestCerts *testhelper.TestCerts, ciphers tlsconfig.TLSOption, clientCAFile string) *syslogTCPServer {
	lis, err := net.Listen("tcp", "127.0.0.1:")
	Expect(err).ToNot(HaveOccurred())
	serverOptions := []tlsconfig.ServerOption{}
	if clientCAFile != "" {
		serverOptions = append(serverOptions, tlsconfig.WithClientAuthenticationFromFile(clientCAFile))
	}

	tlsConfig, err := tlsconfig.Build(
		ciphers,
		tlsconfig.WithIdentityFromFile(
			syslogServerTestCerts.Cert("localhost"),
			syslogServerTestCerts.Key("localhost"),
		),
	).Server(
		serverOptions...,
	)
	if err != nil {
		panic(err)
	}
	tlsLis := tls.NewListener(lis, tlsConfig)
	m := &syslogTCPServer{
		receivedMessages: make(chan *rfc5424.Message, 100),
		lis:              tlsLis,
	}
	go m.accept()
	return m
}

func (m *syslogTCPServer) accept() {
	for {
		conn, err := m.lis.Accept()
		if err != nil {
			return
		}
		go m.handleConn(conn)
	}
}

func (m *syslogTCPServer) handleConn(conn net.Conn) {
	for {
		var msg rfc5424.Message

		_, err := msg.ReadFrom(conn)
		if err != nil {
			return
		}

		m.receivedMessages <- &msg
	}
}

func (m *syslogTCPServer) port() string {
	tokens := strings.Split(m.lis.Addr().String(), ":")
	return tokens[len(tokens)-1]
}
