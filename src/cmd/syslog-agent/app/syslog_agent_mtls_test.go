package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"

	"code.cloudfoundry.org/go-loggregator/v9/rfc5424"
	metricsHelpers "code.cloudfoundry.org/go-metric-registry/testhelpers"
	"code.cloudfoundry.org/loggregator-agent-release/src/cmd/syslog-agent/app"
	"code.cloudfoundry.org/loggregator-agent-release/src/internal/testhelper"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/binding"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/config"
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
			syslogServerCA     []byte

			grpcPort   = 50000
			testLogger = log.New(GinkgoWriter, "", log.LstdFlags)

			metronTestCerts           = testhelper.GenerateCerts("loggregatorCA")
			bindingCacheTestCerts     = testhelper.GenerateCerts("bindingCacheCA")
			syslogServerTestCerts     = testhelper.GenerateCerts("syslogDrainServerCA")
			drainCredentials          = newCredentials("syslogClientCA", "localhost")
			untrustedDrainCredentials = newCredentials("untrustedSyslogCA", "unknown-localhost")
		)

		BeforeEach(func() {
			syslogHTTPS = newSyslogHTTPSServer(syslogServerTestCerts, drainCredentials.caFileName)
			syslogTLS = newSyslogTLSServer(syslogServerTestCerts,
				tlsconfig.WithInternalServiceDefaults(),
				drainCredentials.caFileName,
			)

			aggregateSyslogTLS = newSyslogTLSServer(syslogServerTestCerts, tlsconfig.WithInternalServiceDefaults(), "")
			aggregateAddr = fmt.Sprintf("syslog-tls://127.0.0.1:%s", aggregateSyslogTLS.port())

			var err error
			syslogServerCA, err = os.ReadFile(syslogServerTestCerts.CA())
			Expect(err).ToNot(HaveOccurred())
			bindingCache = &fakeBindingCache{
				bindings: []binding.Binding{
					{
						Url: syslogHTTPS.server.URL,
						Credentials: []binding.Credentials{
							{Cert: drainCredentials.cert, Key: drainCredentials.key, CA: string(syslogServerCA), Apps: []binding.App{{Hostname: "org.space.name", AppID: "some-id"}}}},
					},
					{
						Url: fmt.Sprintf("syslog-tls://localhost:%s", syslogTLS.port()),
						Credentials: []binding.Credentials{
							{
								Cert: drainCredentials.cert, Key: drainCredentials.key, CA: string(syslogServerCA), Apps: []binding.App{{Hostname: "org.space.name", AppID: "some-id-tls"}},
							},
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

		It("egresses logs", func() {
			cancel := setupTestAgent()
			defer cancel()

			var msg *rfc5424.Message
			Eventually(syslogHTTPS.receivedMessages, 3).Should(Receive(&msg))
			Expect(msg.StructuredData).ToNot(HaveLen(0))
			Expect(msg.StructuredData[0].ID).To(Equal("tags@47450"))
			Expect(string(msg.Message)).To(Equal("hello\n"))

			Eventually(syslogTLS.receivedMessages, 3).Should(Receive(&msg))
			Expect(msg.StructuredData).ToNot(HaveLen(0))
			Expect(msg.StructuredData[0].ID).To(Equal("tags@47450"))
			Expect(string(msg.Message)).To(Equal("hello\n"))
		})

		It("will not be trusted if client certs are invalid for server", func() {
			bindingCache.bindings = []binding.Binding{
				{
					Url: fmt.Sprintf("syslog-tls://localhost:%s", syslogTLS.port()),
					Credentials: []binding.Credentials{
						{
							Cert: untrustedDrainCredentials.cert, Key: untrustedDrainCredentials.key, CA: string(syslogServerCA), Apps: []binding.App{{Hostname: "org.space.name", AppID: "some-id-tls"}},
						},
					},
				},
			}
			cancel := setupTestAgent()
			defer cancel()

			var msg *rfc5424.Message
			Consistently(syslogTLS.receivedMessages, 3).ShouldNot(Receive(&msg))
		})

		It("will not accept untrusted server", func() {
			untrustedCA, err := os.ReadFile(untrustedDrainCredentials.caFileName)
			Expect(err).ToNot(HaveOccurred())
			bindingCache.bindings = []binding.Binding{
				{
					Url: fmt.Sprintf("syslog-tls://localhost:%s", syslogTLS.port()),
					Credentials: []binding.Credentials{
						{
							Cert: drainCredentials.cert, Key: drainCredentials.key, CA: string(untrustedCA), Apps: []binding.App{{Hostname: "org.space.name", AppID: "some-id-tls"}},
						},
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
					Url: fmt.Sprintf("syslog-tls://localhost:%s", syslogTLS.port()),
					Credentials: []binding.Credentials{
						{
							Cert: "a cert that is not a cert", Key: "a key that is not a key", Apps: []binding.App{{Hostname: "org.space.name", AppID: "some-id-tls"}},
						},
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

		It("will not activate drains with invalid ca", func() {
			bindingCache.bindings = []binding.Binding{
				{
					Url: fmt.Sprintf("syslog-tls://localhost:%s", syslogTLS.port()),
					Credentials: []binding.Credentials{
						{
							CA: "a cert that is not a cert", Apps: []binding.App{{Hostname: "org.space.name", AppID: "some-id-tls"}},
						},
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

	_, _ = w.Write(results)
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

	cert, err := os.ReadFile(certFileName)
	if err != nil {
		return nil
	}
	key, err := os.ReadFile(keyFileName)
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
