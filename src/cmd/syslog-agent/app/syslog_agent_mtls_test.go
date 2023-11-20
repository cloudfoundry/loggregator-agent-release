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

	"code.cloudfoundry.org/go-loggregator/v9/rfc5424"
	metricsHelpers "code.cloudfoundry.org/go-metric-registry/testhelpers"
	"code.cloudfoundry.org/loggregator-agent-release/src/cmd/syslog-agent/app"
	"code.cloudfoundry.org/loggregator-agent-release/src/internal/testhelper"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/binding"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/config"
	"code.cloudfoundry.org/tlsconfig"
)

var _ = Describe("SyslogAgent with mTLS", func() {
	var (
		grpcPort    int
		pprofPort   int
		metricsPort int

		appHTTPSDrain          *syslogHTTPSServer
		appTLSDrain            *syslogTCPServer
		aggregateDrain         *syslogTCPServer
		aggregateDrainNoClient *syslogTCPServer
		appIDs                 []string
		cacheCerts             *testhelper.TestCerts
		bindingCache           *fakeBindingCache

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

		bindingCreds := newCredentials("syslogClientCA", "localhost")

		drainCerts := testhelper.GenerateCerts("drain-ca")
		appHTTPSDrain = newSyslogHTTPSServer(drainCerts, bindingCreds.caFileName)
		appTLSDrain = newSyslogTLSServer(drainCerts, tlsconfig.WithInternalServiceDefaults(), bindingCreds.caFileName)
		aggregateDrain = newSyslogTLSServer(drainCerts, tlsconfig.WithInternalServiceDefaults(), bindingCreds.caFileName)
		aggregateDrainNoClient = newSyslogTLSServer(drainCerts, tlsconfig.WithInternalServiceDefaults(), "")

		appIDs = []string{"app-1", "app-2"}
		cacheCerts = testhelper.GenerateCerts("binding-cache-ca")
		drainCA, err := os.ReadFile(drainCerts.CA())
		Expect(err).NotTo(HaveOccurred())
		bindingCache = &fakeBindingCache{
			bindings: []binding.Binding{
				{
					Url: appHTTPSDrain.server.URL,
					Credentials: []binding.Credentials{
						{
							Cert: bindingCreds.cert,
							Key:  bindingCreds.key,
							CA:   string(drainCA),
							Apps: []binding.App{
								{
									Hostname: fmt.Sprintf("%s.example.com", appIDs[0]),
									AppID:    appIDs[0],
								},
							},
						},
					},
				},
				{
					Url: fmt.Sprintf("syslog-tls://localhost:%s", appTLSDrain.port()),
					Credentials: []binding.Credentials{
						{
							Cert: bindingCreds.cert,
							Key:  bindingCreds.key,
							CA:   string(drainCA),
							Apps: []binding.App{
								{
									Hostname: fmt.Sprintf("%s.example.com", appIDs[1]),
									AppID:    appIDs[1],
								},
							},
						},
					},
				},
			},
			aggregate: []binding.Binding{
				{
					Url: fmt.Sprintf("syslog-tls://localhost:%s", aggregateDrain.port()),
					Credentials: []binding.Credentials{
						{
							Cert: bindingCreds.cert,
							Key:  bindingCreds.key,
							CA:   string(drainCA),
						},
					},
				},
				{
					Url: fmt.Sprintf("syslog-tls://localhost:%s", aggregateDrainNoClient.port()),
					Credentials: []binding.Credentials{
						{
							CA: string(drainCA),
						},
					},
				},
			},
		}

		agentCerts = testhelper.GenerateCerts("metron-ca")
		agentCfg = app.Config{
			AggregateConnectionRefreshInterval: 1 * time.Minute,
			BindingsPerAppLimit:                5,

			DefaultDrainMetadata: true,
			DrainTrustedCAFile:   drainCerts.CA(),
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
		aggregateDrainNoClient.lis.Close()
		appTLSDrain.lis.Close()
		appHTTPSDrain.server.Close()
	})

	It("connects to drains", func() {
		ctx, cancel := context.WithCancel(context.Background())
		emitLogs(ctx, appIDs, grpcPort, agentCerts)
		defer cancel()

		Eventually(func() float64 {
			return agentMetrics.GetMetric("active_drains", map[string]string{"unit": "count"}).Value()
		}, 3).Should(Equal(4.0))

		var msg *rfc5424.Message

		Eventually(func() float64 {
			return agentMetrics.GetMetric("aggregate_drains", map[string]string{"unit": "count"}).Value()
		}, 3).Should(Equal(2.0))
		Eventually(aggregateDrain.receivedMessages, 3).Should(Receive(&msg))
		Expect(msg.StructuredData).NotTo(HaveLen(0))
		Expect(msg.StructuredData[0].ID).To(Equal("tags@47450"))
		Expect(string(msg.Message)).To(Equal("hello\n"))

		Eventually(aggregateDrainNoClient.receivedMessages, 3).Should(Receive(&msg))
		Expect(msg.StructuredData).NotTo(HaveLen(0))
		Expect(msg.StructuredData[0].ID).To(Equal("tags@47450"))
		Expect(string(msg.Message)).To(Equal("hello\n"))

		Eventually(func() float64 {
			return agentMetrics.GetMetric("drains", map[string]string{"unit": "count"}).Value()
		}, 3).Should(Equal(2.0))
		Eventually(appHTTPSDrain.receivedMessages, 3).Should(Receive(&msg))
		Expect(msg.StructuredData).NotTo(HaveLen(0))
		Expect(msg.StructuredData[0].ID).To(Equal("tags@47450"))
		Expect(string(msg.Message)).To(Equal("hello\n"))
		Eventually(appTLSDrain.receivedMessages, 3).Should(Receive(&msg))
		Expect(msg.StructuredData).NotTo(HaveLen(0))
		Expect(msg.StructuredData[0].ID).To(Equal("tags@47450"))
		Expect(string(msg.Message)).To(Equal("hello\n"))
	})

	Context("when the client certs associated with a drain are not configured on that drain", func() {
		BeforeEach(func() {
			untrustedCerts := newCredentials("untrustedSyslogCA", "unknown-localhost")
			bindingCache.bindings[0].Credentials[0].Cert = untrustedCerts.cert
			bindingCache.bindings[0].Credentials[0].Key = untrustedCerts.key
			bindingCache.bindings[1].Credentials[0].Cert = untrustedCerts.cert
			bindingCache.bindings[1].Credentials[0].Key = untrustedCerts.key
		})

		It("will not be able to connect with those drains", func() {
			ctx, cancel := context.WithCancel(context.Background())
			emitLogs(ctx, appIDs, grpcPort, agentCerts)
			defer cancel()

			Consistently(appHTTPSDrain.receivedMessages, 5).ShouldNot(Receive())
			Consistently(appTLSDrain.receivedMessages, 5).ShouldNot(Receive())
		})
	})

	Context("when a binding CA does not match the actual CA of the drain", func() {
		BeforeEach(func() {
			untrustedCerts := testhelper.GenerateCerts("untrusted")
			bindingCache.bindings[0].Credentials[0].CA = untrustedCerts.CA()
			bindingCache.bindings[1].Credentials[0].CA = untrustedCerts.CA()
		})

		It("refuses to connect to the drain", func() {
			ctx, cancel := context.WithCancel(context.Background())
			emitLogs(ctx, appIDs, grpcPort, agentCerts)
			defer cancel()

			Consistently(appHTTPSDrain.receivedMessages, 5).ShouldNot(Receive())
			Consistently(appTLSDrain.receivedMessages, 5).ShouldNot(Receive())
		})
	})

	Context("when a binding's credentials are invalid", func() {
		BeforeEach(func() {
			bindingCache.bindings[0].Credentials[0].Cert = "invalid"
			bindingCache.bindings[0].Credentials[0].Key = "invalid"
			bindingCache.bindings[1].Credentials[0].CA = "invalid"
		})

		It("does not consider that binding an active drain", func() {
			ctx, cancel := context.WithCancel(context.Background())
			emitLogs(ctx, appIDs, grpcPort, agentCerts)
			defer cancel()

			// The aggregate drain is still active.
			Eventually(func() float64 {
				return agentMetrics.GetMetric("active_drains", map[string]string{"unit": "count"}).Value()
			}, 3).Should(Equal(2.0))

			Consistently(appHTTPSDrain.receivedMessages, 5).ShouldNot(Receive())
			Consistently(appTLSDrain.receivedMessages, 5).ShouldNot(Receive())
		})
	})
})

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
	var (
		results []byte
		err     error
	)

	switch r.URL.Path {
	case "/v2/bindings":
		results, err = json.Marshal(f.bindings)
	case "/v2/aggregate":
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
