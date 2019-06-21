package app_test

import (
	"code.cloudfoundry.org/loggregator-agent/pkg/ingress/cups"
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
	"sync"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"

	"code.cloudfoundry.org/go-loggregator"
	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	"code.cloudfoundry.org/loggregator-agent/cmd/syslog-agent/app"
	"code.cloudfoundry.org/loggregator-agent/internal/testhelper"
	"code.cloudfoundry.org/loggregator-agent/pkg/binding"
	"code.cloudfoundry.org/loggregator-agent/pkg/plumbing"
	"code.cloudfoundry.org/rfc5424"
)

var _ = Describe("SyslogAgent", func() {
	var (
		syslogHTTPS  *syslogHTTPSServer
		universalSyslogTLS *syslogTCPServer
		universalAddr string
		syslogTLS    *syslogTCPServer
		cupsProvider *fakeBindingCache
		metricClient *testhelper.SpyMetricClient

		grpcPort   = 30000
		testLogger = log.New(GinkgoWriter, "", log.LstdFlags)
	)

	BeforeEach(func() {
		syslogHTTPS = newSyslogHTTPSServer()
		syslogTLS = newSyslogTLSServer()

		universalSyslogTLS = newSyslogTLSServer()
		universalAddr = fmt.Sprintf("syslog-tls://127.0.0.1:%s", universalSyslogTLS.port())

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
		cupsProvider.startTLS()
	})

	AfterEach(func() {
		gexec.CleanupBuildArtifacts()
		grpcPort++
	})

	It("has a health endpoint", func() {
		mc := testhelper.NewMetricClient()
		cfg := app.Config{
			BindingsPerAppLimit: 5,
			DebugPort:           7392,
			IdleDrainTimeout:    10 * time.Minute,
			Cache: app.Cache{
				URL:             cupsProvider.URL,
				CAFile:          testhelper.Cert("binding-cache-ca.crt"),
				CertFile:        testhelper.Cert("binding-cache-ca.crt"),
				KeyFile:         testhelper.Cert("binding-cache-ca.key"),
				CommonName:      "bindingCacheCA",
				PollingInterval: 10 * time.Millisecond,
			},
			GRPC: app.GRPC{
				Port:     grpcPort,
				CAFile:   testhelper.Cert("loggregator-ca.crt"),
				CertFile: testhelper.Cert("metron.crt"),
				KeyFile:  testhelper.Cert("metron.key"),
			},
		}
		go app.NewSyslogAgent(cfg, mc, testLogger).Run()

		Eventually(hasMetric(mc, "dropped", map[string]string{"direction": "ingress"})).Should(BeTrue())
		Eventually(hasMetric(mc, "ingress", map[string]string{"scope": "agent"})).Should(BeTrue())
		Eventually(hasMetric(mc, "drains", map[string]string{"unit": "count"})).Should(BeTrue())
		Eventually(hasMetric(mc, "non_app_drains", map[string]string{"unit": "count"})).Should(BeTrue())
		Eventually(hasMetric(mc, "active_drains", map[string]string{"unit": "count"})).Should(BeTrue())
		Eventually(hasMetric(mc, "binding_refresh_count", nil)).Should(BeTrue())
		Eventually(hasMetric(mc, "latency_for_last_binding_refresh", map[string]string{"unit": "ms"})).Should(BeTrue())
		Eventually(hasMetric(mc, "ingress", map[string]string{"scope": "all_drains"})).Should(BeTrue())

		Eventually(hasMetric(mc, "dropped", map[string]string{"direction": "egress"})).Should(BeTrue())
		Eventually(hasMetric(mc, "egress", nil)).Should(BeTrue())
	})

	var setupTestAgent = func(blacklist cups.BlacklistRanges, universalDrains []string) context.CancelFunc{
		metricClient = testhelper.NewMetricClient()
		cfg := app.Config{
			BindingsPerAppLimit: 5,
			DebugPort:           7392,
			IdleDrainTimeout:    10 * time.Minute,
			DrainSkipCertVerify: true,
			Cache: app.Cache{
				URL:             cupsProvider.URL,
				CAFile:          testhelper.Cert("binding-cache-ca.crt"),
				CertFile:        testhelper.Cert("binding-cache-ca.crt"),
				KeyFile:         testhelper.Cert("binding-cache-ca.key"),
				CommonName:      "bindingCacheCA",
				PollingInterval: 10 * time.Millisecond,
				Blacklist:       blacklist,
			},
			GRPC: app.GRPC{
				Port:     grpcPort,
				CAFile:   testhelper.Cert("loggregator-ca.crt"),
				CertFile: testhelper.Cert("metron.crt"),
				KeyFile:  testhelper.Cert("metron.key"),
			},
			UniversalDrainURLs: universalDrains,
		}
		go app.NewSyslogAgent(cfg, metricClient, testLogger).Run()
		ctx, cancel := context.WithCancel(context.Background())
		emitLogs(ctx, grpcPort)

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

	It("should create connections to universal drains", func() {
		cancel := setupTestAgent(cups.BlacklistRanges{}, []string{universalAddr})
		defer cancel()

		Eventually(hasMetric(metricClient, "non_app_drains", map[string]string{"unit": "count"})).Should(BeTrue())
		Expect(metricClient.GetMetric("non_app_drains", map[string]string{"unit": "count"}).Value()).To(Equal(1.0))
		Expect(metricClient.GetMetric("active_drains", map[string]string{"unit": "count"}).Value()).To(Equal(1.0))
	})

	It("egresses logs", func() {
		cancel := setupTestAgent(cups.BlacklistRanges{}, []string{universalAddr})
		defer cancel()

		Eventually(syslogHTTPS.receivedMessages, 5).Should(Receive())
		Eventually(universalSyslogTLS.receivedMessages, 5).Should(Receive())
	})
})

func emitLogs(ctx context.Context, grpcPort int) {
	tlsConfig, err := loggregator.NewIngressTLSConfig(
		testhelper.Cert("loggregator-ca.crt"),
		testhelper.Cert("metron.crt"),
		testhelper.Cert("metron.key"),
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

func hasMetric(mc *testhelper.SpyMetricClient, metricName string, tags map[string]string) func() bool {
	return func() bool {
		return mc.HasMetric(metricName, tags)
	}
}

type fakeBindingCache struct {
	*httptest.Server
	called  bool
	results []binding.Binding
}

func (f *fakeBindingCache) startTLS() {
	tlsConfig, err := plumbing.NewServerMutualTLSConfig(
		testhelper.Cert("binding-cache-ca.crt"),
		testhelper.Cert("binding-cache-ca.key"),
		testhelper.Cert("binding-cache-ca.crt"),
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

func newSyslogHTTPSServer() *syslogHTTPSServer {
	syslogServer := syslogHTTPSServer{
		receivedMessages: make(chan *rfc5424.Message, 100),
	}

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		msg := &rfc5424.Message{}

		data, err := ioutil.ReadAll(r.Body)
		if err != nil {
			panic(err)
		}

		err = msg.UnmarshalBinary(data)
		if err != nil {
			panic(err)
		}

		// msg.AppName
		// msg.MessageID
		syslogServer.receivedMessages <- msg
	}))

	syslogServer.server = server
	return &syslogServer
}

type syslogTCPServer struct {
	lis              net.Listener
	mu               sync.Mutex
	receivedMessages chan *rfc5424.Message
}

func newSyslogTLSServer() *syslogTCPServer {
	lis, err := net.Listen("tcp", ":0")
	Expect(err).ToNot(HaveOccurred())
	cert, err := tls.LoadX509KeyPair(
		testhelper.Cert("forwarder.crt"),
		testhelper.Cert("forwarder.key"),
	)
	Expect(err).ToNot(HaveOccurred())
	tlsLis := tls.NewListener(lis, &tls.Config{
		Certificates: []tls.Certificate{cert},
	})
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

func (m *syslogTCPServer) addr() net.Addr {
	return m.lis.Addr()
}

func (m *syslogTCPServer) port() string {
	tokens := strings.Split(m.lis.Addr().String(), ":")
	return tokens[len(tokens)-1]
}
