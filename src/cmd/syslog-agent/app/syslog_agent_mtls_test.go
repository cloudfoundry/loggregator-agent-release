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
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"

	"code.cloudfoundry.org/go-loggregator/v9/rfc5424"
	metricsHelpers "code.cloudfoundry.org/go-metric-registry/testhelpers"
	"code.cloudfoundry.org/loggregator-agent-release/src/cmd/syslog-agent/app"
	"code.cloudfoundry.org/loggregator-agent-release/src/internal/testhelper"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/binding"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/config"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/ingress/bindings"
	"code.cloudfoundry.org/tlsconfig"
)

var _ = Describe("SyslogAgent supporting mtls", func() {
	Context("when binding cache is configured", func() {
		var (
			syslogHTTPS        *syslogHTTPSServer
			aggregateSyslogTLS *syslogTCPServer
			aggregateAddr      string
			syslogTLS          *syslogTCPServer
			bindingCache       *fakeBindingCache
			metricClient       *metricsHelpers.SpyMetricsRegistry

			grpcPort   = 50000
			testLogger = log.New(GinkgoWriter, "", log.LstdFlags)

			metronTestCerts           = testhelper.GenerateCerts("loggregatorCA")
			bindingCacheTestCerts     = testhelper.GenerateCerts("bindingCacheCA")
			syslogServerTestCerts     = testhelper.GenerateCerts("syslogCA")
			drainCredentials          = newCredentials("syslogCA", "localhost")
			untrustedDrainCredentials = newCredentials("untrustedSyslogCA", "unknown-localhost")
		)

		BeforeEach(func() {
			syslogHTTPS = newSyslogHTTPSServer(syslogServerTestCerts)
			syslogTLS = newSyslogmTLSServer(syslogServerTestCerts,
				tlsconfig.WithInternalServiceDefaults(),
				drainCredentials.caFileName)

			aggregateSyslogTLS = newSyslogTLSServer(syslogServerTestCerts, tlsconfig.WithInternalServiceDefaults())
			aggregateAddr = fmt.Sprintf("syslog-tls://127.0.0.1:%s", aggregateSyslogTLS.port())

			bindingCache = &fakeBindingCache{
				bindings: []binding.Binding{
					{
						Url: syslogHTTPS.server.URL,
						Apps: []binding.App{
							{Hostname: "org.space.name", AppID: "some-id"},
						},
					},
					{
						Url:  fmt.Sprintf("syslog-tls://localhost:%s", syslogTLS.port()),
						Cert: drainCredentials.cert,
						Key:  drainCredentials.key,
						Apps: []binding.App{
							{Hostname: "org.space.name", AppID: "some-id-tls"},
						},
					},
				},
				aggregate: []binding.LegacyBinding{
					{
						AppID: "",
						Drains: []string{
							aggregateAddr,
						},
					},
				},
			}
			bindingCache.startTLS(bindingCacheTestCerts)
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
					URL:             bindingCache.URL,
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
			syslogAgent := app.NewSyslogAgent(cfg, mc, testLogger)
			go syslogAgent.Run()
			defer syslogAgent.Stop()

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

		It("does not have debug metrics by default", func() {
			mc := metricsHelpers.NewMetricsRegistry()
			cfg := app.Config{
				BindingsPerAppLimit: 5,
				MetricsServer: config.MetricsServer{
					Port:      7392,
					CAFile:    metronTestCerts.CA(),
					CertFile:  metronTestCerts.Cert("metron"),
					KeyFile:   metronTestCerts.Key("metron"),
					PprofPort: 1234,
				},
				IdleDrainTimeout: 10 * time.Minute,
				Cache: app.Cache{
					URL:             bindingCache.URL,
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
				LegacyBehaviour:                    false,
			}
			syslogAgent := app.NewSyslogAgent(cfg, mc, testLogger)
			go syslogAgent.Run()
			defer syslogAgent.Stop()

			Consistently(mc.GetDebugMetricsEnabled()).Should(BeFalse())
			Consistently(func() error {
				_, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/debug/pprof/", cfg.MetricsServer.PprofPort))
				return err
			}).ShouldNot(BeNil())
		})

		It("can enabled default metrics", func() {
			mc := metricsHelpers.NewMetricsRegistry()
			cfg := app.Config{
				BindingsPerAppLimit: 5,
				MetricsServer: config.MetricsServer{
					Port:         7392,
					CAFile:       metronTestCerts.CA(),
					CertFile:     metronTestCerts.Cert("metron"),
					KeyFile:      metronTestCerts.Key("metron"),
					PprofPort:    1235,
					DebugMetrics: true,
				},
				IdleDrainTimeout: 10 * time.Minute,
				Cache: app.Cache{
					URL:             bindingCache.URL,
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
				LegacyBehaviour:                    false,
			}
			syslogAgent := app.NewSyslogAgent(cfg, mc, testLogger)
			go syslogAgent.Run()
			defer syslogAgent.Stop()

			Eventually(mc.GetDebugMetricsEnabled).Should(BeTrue())
			var resp *http.Response
			Eventually(func() error {
				var err error
				resp, err = http.Get(fmt.Sprintf("http://127.0.0.1:%d/debug/pprof/", cfg.MetricsServer.PprofPort))
				return err
			}).Should(BeNil())
			Expect(resp.StatusCode).To(Equal(200))
		})

		var setupTestAgent = func(changeConfig ...func(app.Config) app.Config) context.CancelFunc {
			metricClient = metricsHelpers.NewMetricsRegistry()
			cfg := app.Config{
				BindingsPerAppLimit: 5,
				MetricsServer: config.MetricsServer{
					Port:     7392,
					CAFile:   metronTestCerts.CA(),
					CertFile: metronTestCerts.Cert("metron"),
					KeyFile:  metronTestCerts.Key("metron"),
				},
				DefaultDrainMetadata: true,
				IdleDrainTimeout:     10 * time.Minute,
				DrainSkipCertVerify:  false,
				DrainTrustedCAFile:   syslogServerTestCerts.CA(),
				Cache: app.Cache{
					URL:             bindingCache.URL,
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
				LegacyBehaviour:                    false,
			}
			for _, i := range changeConfig {
				cfg = i(cfg)
			}
			syslogAgent := app.NewSyslogAgent(cfg, metricClient, testLogger)
			go syslogAgent.Run()
			defer syslogAgent.Stop()
			ctx, cancel := context.WithCancel(context.Background())
			emitLogs(ctx, grpcPort, metronTestCerts)

			return cancel
		}

		It("should not send logs to blacklisted IPs", func() {
			url, err := url.Parse(syslogHTTPS.server.URL)
			Expect(err).ToNot(HaveOccurred())

			cancel := setupTestAgent(func(config app.Config) app.Config {
				config.Cache.Blacklist = bindings.BlacklistRanges{
					Ranges: []bindings.BlacklistRange{
						{
							Start: url.Hostname(),
							End:   url.Hostname(),
						},
					},
				}
				return config
			},
			)
			defer cancel()

			Consistently(syslogHTTPS.receivedMessages, 3).ShouldNot(Receive())
		})

		It("should create connections to aggregate drains", func() {
			cancel := setupTestAgent()
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
			cancel := setupTestAgent()
			defer cancel()

			var msg *rfc5424.Message
			Eventually(syslogHTTPS.receivedMessages, 3).Should(Receive(&msg))
			Expect(msg.StructuredData).ToNot(HaveLen(0))
			Expect(msg.StructuredData[0].ID).To(Equal("tags@47450"))
			Expect(string(msg.Message)).To(Equal("hello\n"))

			Eventually(aggregateSyslogTLS.receivedMessages, 3).Should(Receive(&msg))
			Expect(msg.StructuredData).ToNot(HaveLen(0))
			Expect(msg.StructuredData[0].ID).To(Equal("tags@47450"))

			Eventually(syslogTLS.receivedMessages, 3).Should(Receive(&msg))
			Expect(msg.StructuredData).ToNot(HaveLen(0))
			Expect(msg.StructuredData[0].ID).To(Equal("tags@47450"))
			Expect(string(msg.Message)).To(Equal("hello\n"))
		})

		It("can be configured so that there's no tags", func() {
			bindingCache.bindings = []binding.Binding{
				{
					Url: fmt.Sprintf("%s?disable-metadata=true", syslogHTTPS.server.URL),
					Apps: []binding.App{
						{Hostname: "org.space.name", AppID: "some-id"},
					},
				},
				{
					Url:  fmt.Sprintf("syslog-tls://localhost:%s?disable-metadata=true", syslogTLS.port()),
					Cert: drainCredentials.cert,
					Key:  drainCredentials.key,
					Apps: []binding.App{
						{Hostname: "org.space.name", AppID: "some-id-tls"},
					},
				},
			}
			bindingCache.aggregate = []binding.LegacyBinding{
				{
					Drains: []string{
						aggregateAddr + "?disable-metadata=true",
					},
				},
			}

			cancel := setupTestAgent()
			defer cancel()

			var msg *rfc5424.Message
			Eventually(syslogHTTPS.receivedMessages, 3).Should(Receive(&msg))
			Expect(msg.StructuredData).To(HaveLen(0))

			Eventually(syslogTLS.receivedMessages, 3).Should(Receive(&msg))
			Expect(msg.StructuredData).To(HaveLen(0))

			Eventually(aggregateSyslogTLS.receivedMessages, 3).Should(Receive(&msg))
			Expect(msg.StructuredData).To(HaveLen(0))
		})

		It("can be configured so that there's no tags by default", func() {
			bindingCache.bindings = []binding.Binding{
				{
					Url: syslogHTTPS.server.URL,
					Apps: []binding.App{
						{Hostname: "org.space.name", AppID: "some-id"},
					},
				},
				{
					Url:  fmt.Sprintf("syslog-tls://localhost:%s", syslogTLS.port()),
					Cert: drainCredentials.cert,
					Key:  drainCredentials.key,
					Apps: []binding.App{
						{Hostname: "org.space.name", AppID: "some-id-tls"},
					},
				},
			}
			bindingCache.aggregate = []binding.LegacyBinding{
				{
					Drains: []string{
						aggregateAddr,
					},
				},
			}
			cancel := setupTestAgent(func(config app.Config) app.Config {
				config.DefaultDrainMetadata = false
				return config
			})
			defer cancel()

			var msg *rfc5424.Message
			Eventually(syslogHTTPS.receivedMessages, 3).Should(Receive(&msg))
			Expect(msg.StructuredData).To(HaveLen(0))

			Eventually(syslogTLS.receivedMessages, 3).Should(Receive(&msg))
			Expect(msg.StructuredData).To(HaveLen(0))

			Eventually(aggregateSyslogTLS.receivedMessages, 3).Should(Receive(&msg))
			Expect(msg.StructuredData).To(HaveLen(0))
		})

		It("will not accept untrusted certs", func() {
			bindingCache.bindings = []binding.Binding{
				{
					Url:  fmt.Sprintf("syslog-tls://localhost:%s", syslogTLS.port()),
					Cert: untrustedDrainCredentials.cert,
					Key:  untrustedDrainCredentials.key,
					Apps: []binding.App{
						{Hostname: "org.space.name", AppID: "some-id-tls"},
					},
				},
			}
			cancel := setupTestAgent()
			defer cancel()

			var msg *rfc5424.Message
			Consistently(syslogTLS.receivedMessages, 3).ShouldNot(Receive(&msg))
		})

		It("will not accept empty drain certs", func() {
			bindingCache.bindings = []binding.Binding{
				{
					Url: fmt.Sprintf("syslog-tls://localhost:%s", syslogTLS.port()),
					Apps: []binding.App{
						{Hostname: "org.space.name", AppID: "some-id-tls"},
					},
				},
			}
			cancel := setupTestAgent()
			defer cancel()

			var msg *rfc5424.Message
			Consistently(syslogTLS.receivedMessages, 3).ShouldNot(Receive(&msg))
		})

		It("will not activate drains with invalid certs", func() {
			bindingCache.bindings = []binding.Binding{
				{
					Url:  fmt.Sprintf("syslog-tls://localhost:%s", syslogTLS.port()),
					Cert: "a cert that is not a cert",
					Key:  "a key that is not a key",
					Apps: []binding.App{
						{Hostname: "org.space.name", AppID: "some-id-tls"},
					},
				},
			}
			bindingCache.aggregate = []binding.LegacyBinding{}
			cancel := setupTestAgent()
			defer cancel()

			Eventually(hasMetric(metricClient, "active_drains", map[string]string{"unit": "count"})).Should(BeTrue())
			Consistently(func() float64 {
				return metricClient.GetMetric("active_drains", map[string]string{"unit": "count"}).Value()
			}, 2).Should(Equal(0.0))
		})
	})
	Context("TLS cipher tests", func() {
		var grpcPort = 51000
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
				LegacyBehaviour:                    false,
			}
			syslogAgent := app.NewSyslogAgent(cfg, metricClient, testLogger)
			go syslogAgent.Run()
			defer syslogAgent.Stop()
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

			grpcPort   = 60000
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
				LegacyBehaviour:                    false,
			}
			syslogAgent := app.NewSyslogAgent(cfg, metricClient, testLogger)
			go syslogAgent.Run()
			defer syslogAgent.Stop()
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

	Context("When GRPC certs are invalid", func() {
		var (
			metricClient *metricsHelpers.SpyMetricsRegistry

			grpcPort   = 70000
			testLogger = log.New(GinkgoWriter, "", log.LstdFlags)

			metronTestCerts       = testhelper.GenerateCerts("loggregatorCA")
			syslogServerTestCerts = testhelper.GenerateCerts("syslogCA")
		)

		AfterEach(func() {
			gexec.CleanupBuildArtifacts()
			grpcPort++
		})

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
				CAFile:   "invalid",
				CertFile: "invalid",
				KeyFile:  "invalid",
			},
			AggregateDrainURLs:                 []string{},
			AggregateConnectionRefreshInterval: 10 * time.Minute,
			LegacyBehaviour:                    false,
		}

		It("should error when creating the TLS client for the logclient", func() {
			Expect(func() { app.NewSyslogAgent(cfg, metricClient, testLogger) }).To(PanicWith("failed to configure client TLS: \"failed to load keypair: open invalid: no such file or directory\""))
		})
	})
})

type fakeBindingCache struct {
	*httptest.Server
	bindings  []binding.Binding
	aggregate []binding.LegacyBinding
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
	var (
		results []byte
		err     error
	)

	switch r.URL.Path {
	case "/v2/bindings":
		results, err = json.Marshal(f.bindings)
	case "/bindings":
		results, err = json.Marshal(binding.ToLegacyBindings(f.bindings))
	case "/aggregate":
		results, err = json.Marshal(f.aggregate)
	default:
		w.WriteHeader(500)
		return
	}

	if err != nil {
		w.WriteHeader(500)
		return
	}

	w.Write(results)
}

type credentials struct {
	cert         string
	key          string
	certFileName string
	keyFileName  string
	caFileName   string
}

func newCredentials(ca, commonName string) *credentials {
	testCerts := testhelper.GenerateCerts(ca)
	certFileName := testCerts.Cert(commonName)
	keyFileName := testCerts.Key(commonName)

	cert, err := ioutil.ReadFile(certFileName)
	if err != nil {
		return nil
	}
	key, err := ioutil.ReadFile(keyFileName)
	if err != nil {
		return nil
	}

	return &credentials{
		cert:         string(cert),
		key:          string(key),
		certFileName: certFileName,
		keyFileName:  keyFileName,
		caFileName:   testCerts.CA(),
	}
}

func newSyslogmTLSServer(syslogServerTestCerts *testhelper.TestCerts,
	ciphers tlsconfig.TLSOption, caFileName string) *syslogTCPServer {
	lis, err := net.Listen("tcp", ":0")
	Expect(err).ToNot(HaveOccurred())
	pool := tlsconfig.FromEmptyPool(
		tlsconfig.WithCertsFromFile(caFileName),
	)
	tlsConfig, err := tlsconfig.Build(
		ciphers,
		tlsconfig.WithIdentityFromFile(
			syslogServerTestCerts.Cert("localhost"),
			syslogServerTestCerts.Key("localhost"),
		),
		tlsconfig.TLSOption(tlsconfig.WithClientAuthenticationBuilder(pool)),
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
