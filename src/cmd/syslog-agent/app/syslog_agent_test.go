package app_test

import (
	"code.cloudfoundry.org/loggregator-agent/pkg/config"
	"code.cloudfoundry.org/loggregator-agent/pkg/ingress/cups"
	"code.cloudfoundry.org/tlsconfig"
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

	metricsHelpers "code.cloudfoundry.org/go-metric-registry/testhelpers"
	"code.cloudfoundry.org/go-loggregator"
	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	"code.cloudfoundry.org/loggregator-agent/cmd/syslog-agent/app"
	"code.cloudfoundry.org/loggregator-agent/internal/testhelper"
	"code.cloudfoundry.org/loggregator-agent/pkg/binding"
	"code.cloudfoundry.org/rfc5424"
)

var _ = Describe("SyslogAgent", func() {
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
		syslogTLS = newSyslogTLSServer(syslogServerTestCerts)

		aggregateSyslogTLS = newSyslogTLSServer(syslogServerTestCerts)
		aggregateAddr = fmt.Sprintf("syslog-tls://127.0.0.1:%s", aggregateSyslogTLS.port())

		cupsProvider = &fakeBindingCache{
			results: []binding.Binding{
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

	var setupTestAgent = func(blacklist cups.BlacklistRanges, aggregateDrains []string) context.CancelFunc {
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
			AggregateDrainURLs: aggregateDrains,
		}
		go app.NewSyslogAgent(cfg, metricClient, testLogger).Run()
		ctx, cancel := context.WithCancel(context.Background())
		emitLogs(ctx, grpcPort, metronTestCerts)

		return cancel
	}

	It("should not send logs to blacklisted IPs", func() {
		url, err := url.Parse(syslogHTTPS.server.URL)
		Expect(err).ToNot(HaveOccurred())

		cancel := setupTestAgent(cups.BlacklistRanges{
			Ranges: []cups.BlacklistRange{
				{
					Start: url.Hostname(),
					End:   url.Hostname(),
				},
			},
		}, nil)
		defer cancel()

		Consistently(syslogHTTPS.receivedMessages, 5).ShouldNot(Receive())
	})

	It("should create connections to aggregate drains", func() {
		cancel := setupTestAgent(cups.BlacklistRanges{}, []string{aggregateAddr})
		defer cancel()

		Eventually(hasMetric(metricClient, "aggregate_drains", map[string]string{"unit": "count"})).Should(BeTrue())
		Eventually(func() float64 {
			return metricClient.GetMetric("aggregate_drains", map[string]string{"unit": "count"}).Value()
		}).Should(Equal(1.0))

		// 2 app drains and 1 aggregate drain
		Eventually(func() float64 {
			return metricClient.GetMetric("active_drains", map[string]string{"unit": "count"}).Value()
		}, 5).Should(Equal(3.0))
	})

	It("egresses logs", func() {
		cancel := setupTestAgent(cups.BlacklistRanges{}, []string{aggregateAddr})
		defer cancel()

		Eventually(syslogHTTPS.receivedMessages, 5).Should(Receive())
		Eventually(aggregateSyslogTLS.receivedMessages, 5).Should(Receive())
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
	results []binding.Binding
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
	if r.URL.Path != "/bindings" {
		w.WriteHeader(500)
		return
	}

	f.serveWithResults(w, r)
}

func (f *fakeBindingCache) serveWithResults(w http.ResponseWriter, r *http.Request) {
	resultData, err := json.Marshal(&f.results)
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

func newSyslogTLSServer(syslogServerTestCerts *testhelper.TestCerts) *syslogTCPServer {
	lis, err := net.Listen("tcp", ":0")
	Expect(err).ToNot(HaveOccurred())

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
