package app_test

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"

	"code.cloudfoundry.org/go-loggregator/v8"
	"code.cloudfoundry.org/go-loggregator/v8/rpc/loggregator_v2"
	metricsHelpers "code.cloudfoundry.org/go-metric-registry/testhelpers"
	"code.cloudfoundry.org/loggregator-agent-release/src/cmd/syslog-agent/app"
	"code.cloudfoundry.org/loggregator-agent-release/src/internal/testhelper"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/binding"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/config"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/ingress/bindings"
	"code.cloudfoundry.org/rfc5424"
	"code.cloudfoundry.org/tlsconfig"
)

var _ = Describe("SyslogAgent", func() {
	Context("when binding cache is configured", func() {
		var (
			syslogHTTPS        *syslogHTTPSServer
			aggregateSyslogTLS *syslogTCPServer
			aggregateAddr      string
			syslogTLS          *syslogTCPServer
			cupsProvider       *fakeBindingCache
			metricClient       *metricsHelpers.SpyMetricsRegistry

			grpcPort   = 30000
			testLogger = log.New(GinkgoWriter, "", log.LstdFlags)

			metronTestCerts       = testhelper.GenerateCerts("loggregatorCA")
			bindingCacheTestCerts = testhelper.GenerateCerts("bindingCacheCA")
			syslogServerTestCerts = testhelper.GenerateCerts("syslogCA")
		)

		BeforeEach(func() {
			syslogHTTPS = newSyslogHTTPSServer(syslogServerTestCerts)
			syslogTLS = newSyslogTLSServer(syslogServerTestCerts, tlsconfig.WithInternalServiceDefaults())

			aggregateSyslogTLS = newSyslogTLSServer(syslogServerTestCerts, tlsconfig.WithInternalServiceDefaults())
			aggregateAddr = fmt.Sprintf("syslog-tls://127.0.0.1:%s", aggregateSyslogTLS.port())

			cupsProvider = &fakeBindingCache{
				bindings: []binding.Binding{
					{
						AppID:    "some-id",
						Hostname: "org.space.name",
						Drains: []string{
							syslogHTTPS.server.URL,
						},
					},
					{
						AppID:    "some-id-tls",
						Hostname: "org.space.name",
						Drains: []string{
							fmt.Sprintf("syslog-tls://localhost:%s", syslogTLS.port()),
						},
					},
				},
				aggregate: []binding.Binding{
					{
						AppID: "",
						Drains: []string{
							aggregateAddr,
						},
					},
				},
			}
			cupsProvider.startTLS(bindingCacheTestCerts)
		})

		AfterEach(func() {
			gexec.CleanupBuildArtifacts()
			grpcPort++
		})

		It("has a health endpoint", func() {
			mc := metricsHelpers.NewMetricsRegistry()
			cfg := app.Config{
				BindingsPerAppLimit: 5,
				MetricsServer: config.MetricsServer{
					Port:     7392,
					CAFile:   metronTestCerts.CA(),
					CertFile: metronTestCerts.Cert("metron"),
					KeyFile:  metronTestCerts.Key("metron"),
				},
				IdleDrainTimeout: 10 * time.Minute,
				Cache: app.Cache{
					URL:             cupsProvider.URL,
					CAFile:          bindingCacheTestCerts.CA(),
					CertFile:        bindingCacheTestCerts.Cert("binding-cache"),
					KeyFile:         bindingCacheTestCerts.Key("binding-cache"),
					CommonName:      "binding-cache",
					PollingInterval: 10 * time.Millisecond,
				},
				GRPC: app.GRPC{
					Port:     grpcPort,
					CAFile:   metronTestCerts.CA(),
					CertFile: metronTestCerts.Cert("metron"),
					KeyFile:  metronTestCerts.Key("metron"),
				},
				AggregateConnectionRefreshInterval: 10 * time.Minute,
			}
			go app.NewSyslogAgent(cfg, mc, testLogger).Run()

			Eventually(hasMetric(mc, "dropped", map[string]string{"direction": "ingress"})).Should(BeTrue())
			Eventually(hasMetric(mc, "ingress", map[string]string{"scope": "agent"})).Should(BeTrue())
			Eventually(hasMetric(mc, "drains", map[string]string{"unit": "count"})).Should(BeTrue())
			Eventually(hasMetric(mc, "aggregate_drains", map[string]string{"unit": "count"})).Should(BeTrue())
			Eventually(hasMetric(mc, "active_drains", map[string]string{"unit": "count"})).Should(BeTrue())
			Eventually(hasMetric(mc, "binding_refresh_count", nil)).Should(BeTrue())
			Eventually(hasMetric(mc, "latency_for_last_binding_refresh", map[string]string{"unit": "ms"})).Should(BeTrue())
			Eventually(hasMetric(mc, "ingress", map[string]string{"scope": "all_drains"})).Should(BeTrue())

			Eventually(hasMetric(mc, "dropped", map[string]string{"direction": "egress"})).Should(BeTrue())
			Eventually(hasMetric(mc, "egress", nil)).Should(BeTrue())
		})

		var setupTestAgent = func(blacklist bindings.BlacklistRanges, aggregateDrains []string) context.CancelFunc {
			metricClient = metricsHelpers.NewMetricsRegistry()
			cfg := app.Config{
				BindingsPerAppLimit: 5,
				MetricsServer: config.MetricsServer{
					Port:     7392,
					CAFile:   metronTestCerts.CA(),
					CertFile: metronTestCerts.Cert("metron"),
					KeyFile:  metronTestCerts.Key("metron"),
				},
				IdleDrainTimeout:    10 * time.Minute,
				DrainSkipCertVerify: false,
				DrainTrustedCAFile:  syslogServerTestCerts.CA(),
				Cache: app.Cache{
					URL:             cupsProvider.URL,
					CAFile:          bindingCacheTestCerts.CA(),
					CertFile:        bindingCacheTestCerts.Cert("binding-cache"),
					KeyFile:         bindingCacheTestCerts.Key("binding-cache"),
					CommonName:      "binding-cache",
					PollingInterval: 10 * time.Millisecond,
					Blacklist:       blacklist,
				},
				GRPC: app.GRPC{
					Port:     grpcPort,
					CAFile:   metronTestCerts.CA(),
					CertFile: metronTestCerts.Cert("metron"),
					KeyFile:  metronTestCerts.Key("metron"),
				},
				AggregateDrainURLs:                 aggregateDrains,
				AggregateConnectionRefreshInterval: 10 * time.Minute,
			}
			go app.NewSyslogAgent(cfg, metricClient, testLogger).Run()
			ctx, cancel := context.WithCancel(context.Background())
			emitLogs(ctx, grpcPort, metronTestCerts)

			return cancel
		}

		It("should not send logs to blacklisted IPs", func() {
			url, err := url.Parse(syslogHTTPS.server.URL)
			Expect(err).ToNot(HaveOccurred())

			cancel := setupTestAgent(bindings.BlacklistRanges{
				Ranges: []bindings.BlacklistRange{
					{
						Start: url.Hostname(),
						End:   url.Hostname(),
					},
				},
			},
				nil,
			)
			defer cancel()

			Consistently(syslogHTTPS.receivedMessages, 3).ShouldNot(Receive())
		})

		It("should create connections to aggregate drains", func() {
			cancel := setupTestAgent(bindings.BlacklistRanges{}, []string{})
			defer cancel()

			Eventually(hasMetric(metricClient, "aggregate_drains", map[string]string{"unit": "count"})).Should(BeTrue())
			Eventually(func() float64 {
				return metricClient.GetMetric("aggregate_drains", map[string]string{"unit": "count"}).Value()
			}).Should(Equal(1.0))

			// 2 app drains and 1 aggregate drain
			Eventually(func() float64 {
				return metricClient.GetMetric("active_drains", map[string]string{"unit": "count"}).Value()
			}, 3).Should(Equal(3.0))
		})

		It("egresses logs", func() {
			cancel := setupTestAgent(bindings.BlacklistRanges{}, []string{})
			defer cancel()

			var msg *rfc5424.Message
			Eventually(syslogHTTPS.receivedMessages, 3).Should(Receive(&msg))
			Expect(msg.StructuredData[0].ID).To(Equal("tags@47450"))

			Eventually(aggregateSyslogTLS.receivedMessages, 3).Should(Receive(&msg))
			Expect(msg.StructuredData[0].ID).To(Equal("tags@47450"))
		})

		It("can be configured so that there's no tags", func() {
			cupsProvider.bindings = []binding.Binding{
				{
					AppID:    "some-id",
					Hostname: "org.space.name",
					Drains: []string{
						syslogHTTPS.server.URL + "?disable-metadata=true",
					},
				},
				{
					AppID:    "some-id-tls",
					Hostname: "org.space.name",
					Drains: []string{
						fmt.Sprintf("syslog-tls://localhost:%s?disable-metadata=true", syslogTLS.port()),
					},
				},
			}
			cupsProvider.aggregate = []binding.Binding{
				{
					Drains: []string{
						aggregateAddr + "?disable-metadata=true",
					},
				},
			}
			cancel := setupTestAgent(bindings.BlacklistRanges{}, []string{})
			defer cancel()

			var msg *rfc5424.Message
			Eventually(syslogHTTPS.receivedMessages, 3).Should(Receive(&msg))
			Expect(msg.StructuredData).To(HaveLen(0))

			Eventually(syslogTLS.receivedMessages, 3).Should(Receive(&msg))
			Expect(msg.StructuredData).To(HaveLen(0))

			Eventually(aggregateSyslogTLS.receivedMessages, 3).Should(Receive(&msg))
			Expect(msg.StructuredData).To(HaveLen(0))
		})
	})

	Context("TLS cipher tests", func() {
		var grpcPort = 41000
		var aggregateSyslogTLS *syslogTCPServer

		AfterEach(func() {
			gexec.CleanupBuildArtifacts()
			grpcPort++
		})

		var setupTestAgentAndServerNoBindingCache = func(serverCiphers tlsconfig.TLSOption, agentCiphers string, aggregateQueryParam string) context.CancelFunc {
			syslogServerTestCerts := testhelper.GenerateCerts("syslogCA")
			aggregateSyslogTLS = newSyslogTLSServer(syslogServerTestCerts, serverCiphers)
			aggregateAddr := fmt.Sprintf("syslog-tls://127.0.0.1:%s%s", aggregateSyslogTLS.port(), aggregateQueryParam)

			metronTestCerts := testhelper.GenerateCerts("loggregatorCA")
			metricClient := metricsHelpers.NewMetricsRegistry()
			testLogger := log.New(GinkgoWriter, "", log.LstdFlags)
			cfg := app.Config{
				BindingsPerAppLimit: 5,
				MetricsServer: config.MetricsServer{
					Port:     8052,
					CAFile:   metronTestCerts.CA(),
					CertFile: metronTestCerts.Cert("metron"),
					KeyFile:  metronTestCerts.Key("metron"),
				},
				IdleDrainTimeout:    10 * time.Minute,
				DrainSkipCertVerify: false,
				DrainCipherSuites:   agentCiphers,
				DrainTrustedCAFile:  syslogServerTestCerts.CA(),
				GRPC: app.GRPC{
					Port:     grpcPort,
					CAFile:   metronTestCerts.CA(),
					CertFile: metronTestCerts.Cert("metron"),
					KeyFile:  metronTestCerts.Key("metron"),
				},
				AggregateDrainURLs:                 []string{aggregateAddr},
				AggregateConnectionRefreshInterval: 10 * time.Minute,
			}
			go app.NewSyslogAgent(cfg, metricClient, testLogger).Run()
			ctx, cancel := context.WithCancel(context.Background())
			emitLogs(ctx, grpcPort, metronTestCerts)

			return cancel
		}
		Context("When drain is using default external ciphers", func() {
			It("Can communicate with compatible server", func() {
				cancel := setupTestAgentAndServerNoBindingCache(
					func(c *tls.Config) error {
						c.MinVersion = tls.VersionTLS12
						c.MaxVersion = tls.VersionTLS12
						c.PreferServerCipherSuites = false
						// External ciphers not on internal list
						c.CipherSuites = []uint16{
							tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
							tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
							tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
							tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
						}
						return nil
					},
					"",
					"",
				)

				defer cancel()

				Eventually(aggregateSyslogTLS.receivedMessages, 3).Should(Receive())
			})

			It("refuses to communicate with non-compatible server", func() {
				cancel := setupTestAgentAndServerNoBindingCache(
					func(c *tls.Config) error {
						c.MinVersion = tls.VersionTLS12
						c.MaxVersion = tls.VersionTLS12
						c.PreferServerCipherSuites = false
						// External ciphers not on internal list
						c.CipherSuites = []uint16{
							tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
						}
						return nil
					},
					"",
					"",
				)

				defer cancel()

				Consistently(aggregateSyslogTLS.receivedMessages, 3).ShouldNot(Receive())
			})

		})
		Context("When drain is using overridden external ciphers", func() {
			It("Can communicate with compatible server", func() {

				cancel := setupTestAgentAndServerNoBindingCache(
					func(c *tls.Config) error {
						c.MinVersion = tls.VersionTLS12
						c.MaxVersion = tls.VersionTLS12
						c.PreferServerCipherSuites = false
						// External ciphers not on internal list
						c.CipherSuites = []uint16{
							tls.TLS_RSA_WITH_RC4_128_SHA,
						}
						return nil
					},
					"TLS_RSA_WITH_RC4_128_SHA",
					"",
				)

				defer cancel()

				Eventually(aggregateSyslogTLS.receivedMessages, 3).Should(Receive())
			})
			It("refuses to communicate with non-compatible server", func() {

				cancel := setupTestAgentAndServerNoBindingCache(
					func(c *tls.Config) error {
						c.MinVersion = tls.VersionTLS12
						c.MaxVersion = tls.VersionTLS12
						c.PreferServerCipherSuites = false
						// External ciphers not on internal list
						c.CipherSuites = []uint16{
							tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
						}
						return nil
					},
					"TLS_RSA_WITH_RC4_128_SHA",
					"",
				)

				defer cancel()

				Consistently(aggregateSyslogTLS.receivedMessages, 3).ShouldNot(Receive())
			})
		})

		Context("When drain is using internal ciphers", func() {
			It("Can be scraped by agent using internal ciphers", func() {
				cancel := setupTestAgentAndServerNoBindingCache(
					tlsconfig.WithInternalServiceDefaults(),
					"",
					"?ssl-strict-internal=true",
				)

				defer cancel()

				Eventually(aggregateSyslogTLS.receivedMessages, 3).Should(Receive())
			})
			It("refuses to communicate with non-compatible server", func() {
				cancel := setupTestAgentAndServerNoBindingCache(
					func(c *tls.Config) error {
						c.MinVersion = tls.VersionTLS12
						c.MaxVersion = tls.VersionTLS12
						c.PreferServerCipherSuites = false
						// External ciphers not on internal list
						c.CipherSuites = []uint16{
							tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
							tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
							tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
							tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
						}
						return nil
					},
					"",
					"?ssl-strict-internal=true",
				)

				defer cancel()

				Consistently(aggregateSyslogTLS.receivedMessages, 3).ShouldNot(Receive())
			})
		})

	})

	Context("When binding-cache is not configured", func() {
		var (
			aggregateSyslogTLS *syslogTCPServer
			aggregateAddr      string
			metricClient       *metricsHelpers.SpyMetricsRegistry

			grpcPort   = 40000
			testLogger = log.New(GinkgoWriter, "", log.LstdFlags)

			metronTestCerts       = testhelper.GenerateCerts("loggregatorCA")
			syslogServerTestCerts = testhelper.GenerateCerts("syslogCA")
		)

		BeforeEach(func() {
			aggregateSyslogTLS = newSyslogTLSServer(syslogServerTestCerts, tlsconfig.WithInternalServiceDefaults())
			aggregateAddr = fmt.Sprintf("syslog-tls://127.0.0.1:%s", aggregateSyslogTLS.port())
		})

		AfterEach(func() {
			gexec.CleanupBuildArtifacts()
			grpcPort++
		})

		var setupTestAgentNoBindingCache = func(blacklist bindings.BlacklistRanges, aggregateDrains []string) context.CancelFunc {
			metricClient = metricsHelpers.NewMetricsRegistry()
			cfg := app.Config{
				BindingsPerAppLimit: 5,
				MetricsServer: config.MetricsServer{
					Port:     8052,
					CAFile:   metronTestCerts.CA(),
					CertFile: metronTestCerts.Cert("metron"),
					KeyFile:  metronTestCerts.Key("metron"),
				},
				IdleDrainTimeout:    10 * time.Minute,
				DrainSkipCertVerify: false,
				DrainTrustedCAFile:  syslogServerTestCerts.CA(),
				GRPC: app.GRPC{
					Port:     grpcPort,
					CAFile:   metronTestCerts.CA(),
					CertFile: metronTestCerts.Cert("metron"),
					KeyFile:  metronTestCerts.Key("metron"),
				},
				AggregateDrainURLs:                 aggregateDrains,
				AggregateConnectionRefreshInterval: 10 * time.Minute,
			}
			go app.NewSyslogAgent(cfg, metricClient, testLogger).Run()
			ctx, cancel := context.WithCancel(context.Background())
			emitLogs(ctx, grpcPort, metronTestCerts)

			return cancel
		}

		It("should create connections to aggregate drains", func() {
			cancel := setupTestAgentNoBindingCache(bindings.BlacklistRanges{}, []string{aggregateAddr})
			defer cancel()

			Eventually(hasMetric(metricClient, "aggregate_drains", map[string]string{"unit": "count"})).Should(BeTrue())
			Eventually(func() float64 {
				return metricClient.GetMetric("aggregate_drains", map[string]string{"unit": "count"}).Value()
			}).Should(Equal(1.0))

			// 0 app drains and 1 aggregate drain
			Eventually(func() float64 {
				return metricClient.GetMetric("active_drains", map[string]string{"unit": "count"}).Value()
			}, 3).Should(Equal(1.0))
		})

		It("egresses logs", func() {
			cancel := setupTestAgentNoBindingCache(bindings.BlacklistRanges{}, []string{aggregateAddr})
			defer cancel()

			Eventually(aggregateSyslogTLS.receivedMessages, 3).Should(Receive())
		})
	})
})

func emitLogs(ctx context.Context, grpcPort int, testCerts *testhelper.TestCerts) {
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

	e := &loggregator_v2.Envelope{
		Timestamp: time.Now().UnixNano(),
		SourceId:  "some-id",
		Message: &loggregator_v2.Envelope_Log{
			Log: &loggregator_v2.Log{
				Payload: []byte("hello"),
			},
		},
		Tags: map[string]string{
			"foo": "bar",
		},
	}

	eTLS := &loggregator_v2.Envelope{
		Timestamp: time.Now().UnixNano(),
		SourceId:  "some-id-tls",
		Message: &loggregator_v2.Envelope_Log{
			Log: &loggregator_v2.Log{
				Payload: []byte("hello"),
			},
		},
	}

	go func() {
		ticker := time.NewTicker(10 * time.Millisecond)
		for {
			select {
			case <-ticker.C:
				ingressClient.Emit(e)
				ingressClient.Emit(eTLS)
			case <-ctx.Done():

				return
			}
		}
	}()
}

func hasMetric(mc *metricsHelpers.SpyMetricsRegistry, metricName string, tags map[string]string) func() bool {
	return func() bool {
		return mc.HasMetric(metricName, tags)
	}
}

type fakeBindingCache struct {
	*httptest.Server
	bindings  []binding.Binding
	aggregate []binding.Binding
}

func (f *fakeBindingCache) startTLS(testCerts *testhelper.TestCerts) {
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

func (f *fakeBindingCache) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	results := []binding.Binding{}
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

	w.Write(resultData)
}

type syslogHTTPSServer struct {
	receivedMessages chan *rfc5424.Message
	server           *httptest.Server
}

func newSyslogHTTPSServer(syslogServerTestCerts *testhelper.TestCerts) *syslogHTTPSServer {
	syslogServer := syslogHTTPSServer{
		receivedMessages: make(chan *rfc5424.Message, 100),
	}

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		msg := &rfc5424.Message{}

		data, err := ioutil.ReadAll(r.Body)
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
	).Server()
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

func newSyslogTLSServer(syslogServerTestCerts *testhelper.TestCerts, ciphers tlsconfig.TLSOption) *syslogTCPServer {
	lis, err := net.Listen("tcp", ":0")
	Expect(err).ToNot(HaveOccurred())

	tlsConfig, err := tlsconfig.Build(
		ciphers,
		tlsconfig.WithIdentityFromFile(
			syslogServerTestCerts.Cert("localhost"),
			syslogServerTestCerts.Key("localhost"),
		),
	).Server()
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
