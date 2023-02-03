package app_test

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"time"

	"code.cloudfoundry.org/loggregator-agent-release/src/cmd/prom-scraper/app"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/scraper"
	"github.com/onsi/gomega/gexec"

	"code.cloudfoundry.org/go-loggregator/v9/rpc/loggregator_v2"
	metricsHelpers "code.cloudfoundry.org/go-metric-registry/testhelpers"
	"code.cloudfoundry.org/loggregator-agent-release/src/internal/testhelper"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/plumbing"
	"code.cloudfoundry.org/tlsconfig"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
)

var _ = Describe("PromScraper", func() {
	var (
		spyAgent          *spyAgent
		spyConfigProvider *spyConfigProvider

		cfg          app.Config
		promServer   *stubPromServer
		ps           *app.PromScraper
		metricClient *metricsHelpers.SpyMetricsRegistry

		testLogger  = log.New(GinkgoWriter, "", log.LstdFlags)
		testCerts   = testhelper.GenerateCerts("loggregatorCA")
		scrapeCerts = testhelper.GenerateCerts("scrapeCA")

		metricConfigDir string
	)

	BeforeEach(func() {
		metricConfigDir = metricPortConfigDir()

		spyAgent = newSpyAgent(testCerts)
		spyConfigProvider = newSpyConfigProvider()
		metricClient = metricsHelpers.NewMetricsRegistry()

		cfg = app.Config{
			ClientKeyPath:          testCerts.Key("metron"),
			ClientCertPath:         testCerts.Cert("metron"),
			CACertPath:             testCerts.CA(),
			LoggregatorIngressAddr: spyAgent.addr,
			ConfigGlobs:            []string{fmt.Sprintf("%s/prom_scraper_config*", metricConfigDir)},
			DefaultScrapeInterval:  100 * time.Millisecond,
			SkipSSLValidation:      true,
		}
	})

	AfterEach(func() {
		ps.Stop()

		Eventually(func() error {
			return os.RemoveAll(metricConfigDir)
		}, 10).Should(Succeed())

		gexec.CleanupBuildArtifacts()
	})

	Context("debug metrics", func() {
		It("debug metrics arn't enabled by default", func() {
			cfg.MetricsServer.PprofPort = 1234
			ps = app.NewPromScraper(cfg, spyConfigProvider.Configs, metricClient, testLogger)
			go ps.Run()

			Consistently(metricClient.GetDebugMetricsEnabled()).Should(BeFalse())
			Consistently(func() error {
				_, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/debug/pprof/", cfg.MetricsServer.PprofPort))
				return err
			}).ShouldNot(BeNil())
		})

		It("debug metrics can be enabled", func() {
			cfg.MetricsServer.DebugMetrics = true
			cfg.MetricsServer.PprofPort = 1235
			ps = app.NewPromScraper(cfg, spyConfigProvider.Configs, metricClient, testLogger)
			go ps.Run()

			Eventually(metricClient.GetDebugMetricsEnabled).Should(BeTrue())
			var resp *http.Response
			Eventually(func() error {
				var err error
				resp, err = http.Get(fmt.Sprintf("http://127.0.0.1:%d/debug/pprof/", cfg.MetricsServer.PprofPort))
				return err
			}).Should(BeNil())
			Expect(resp.StatusCode).To(Equal(200))
		})
	})

	Context("http", func() {
		BeforeEach(func() {
			promServer = newStubPromServer()
		})

		It("scrapes a prometheus endpoint and sends those metrics to a loggregator agent", func() {
			promServer.resp = promOutput
			spyConfigProvider.scrapeConfigs = []scraper.PromScraperConfig{{
				Port:       promServer.port,
				SourceID:   "some-id",
				InstanceID: "some-instance-id",
			}}

			ps = app.NewPromScraper(cfg, spyConfigProvider.Configs, metricClient, testLogger)
			go ps.Run()

			Eventually(spyAgent.Envelopes).Should(And(
				ContainElement(buildCounter("test_counter_prometheus_1", "some-id", "some-instance-id", 1)),
				ContainElement(buildGauge("test_gauge_prometheus_1", "some-id", "some-instance-id", 2)),
			))
		})

		It("scrapes prometheus endpoints after the specified interval", func() {
			promServer2 := newStubPromServer()
			spyConfigProvider.scrapeConfigs = []scraper.PromScraperConfig{
				{
					Port:           promServer2.port,
					SourceID:       "some-id",
					InstanceID:     "some-instance-id",
					ScrapeInterval: 100 * time.Millisecond,
				},
				{
					Port:           promServer.port,
					SourceID:       "some-id",
					InstanceID:     "some-instance-id",
					ScrapeInterval: time.Hour,
				},
			}

			promServer.resp = promOutput
			promServer2.resp = promOutput2

			cfg.ConfigGlobs = append(cfg.ConfigGlobs, fmt.Sprintf("%s/metric_port*", metricConfigDir))
			cfg.DefaultScrapeInterval = time.Hour
			ps = app.NewPromScraper(cfg, spyConfigProvider.Configs, metricClient, testLogger)
			go ps.Run()

			Eventually(spyAgent.Envelopes).Should(
				ContainElement(buildCounter("test_counter_prometheus_2", "some-id", "some-instance-id", 3)),
			)

			Consistently(spyAgent.Envelopes).ShouldNot(
				ContainElement(buildCounter("test_counter_prometheus_1", "some-id", "some-instance-id", 1)),
			)
		})

		It("does not scrape endpoints with non-positive scrape timer intervals", func() {
			spyConfigProvider.scrapeConfigs = []scraper.PromScraperConfig{
				{
					Port:           promServer.port,
					SourceID:       "some-id",
					InstanceID:     "some-instance-id",
					ScrapeInterval: -1,
				},
			}

			promServer.resp = promOutput

			cfg.ConfigGlobs = append(cfg.ConfigGlobs, fmt.Sprintf("%s/metric_port*", metricConfigDir))
			ps = app.NewPromScraper(cfg, spyConfigProvider.Configs, metricClient, testLogger)
			go ps.Run()

			Consistently(spyAgent.Envelopes).ShouldNot(
				ContainElement(buildCounter("test_counter_prometheus_1", "some-id", "some-instance-id", 1)),
			)
		})

		It("scrapes multiple prometheus endpoints", func() {
			promServer2 := newStubPromServer()

			spyConfigProvider.scrapeConfigs = []scraper.PromScraperConfig{
				{
					Port:       promServer.port,
					SourceID:   "some-id",
					InstanceID: "some-instance-id",
				},
				{
					Port:       promServer2.port,
					SourceID:   "some-id",
					InstanceID: "some-instance-id",
				},
			}

			promServer.resp = promOutput
			promServer2.resp = promOutput2

			cfg.ConfigGlobs = append(cfg.ConfigGlobs, fmt.Sprintf("%s/metric_port*", metricConfigDir))
			ps = app.NewPromScraper(cfg, spyConfigProvider.Configs, metricClient, testLogger)
			go ps.Run()

			Eventually(spyAgent.Envelopes).Should(And(
				ContainElement(buildCounter("test_counter_prometheus_1", "some-id", "some-instance-id", 1)),
				ContainElement(buildGauge("test_gauge_prometheus_1", "some-id", "some-instance-id", 2)),
				ContainElement(buildCounter("test_counter_prometheus_2", "some-id", "some-instance-id", 3)),
			))
		})

		It("scrapes with headers if provided", func() {
			spyConfigProvider.scrapeConfigs = []scraper.PromScraperConfig{{
				Port:       promServer.port,
				SourceID:   "some-id",
				InstanceID: "some-instance-id",
				Headers: map[string]string{
					"header1": "value1",
					"Header2": "value2",
				},
			}}
			promServer.resp = promOutput

			ps = app.NewPromScraper(cfg, spyConfigProvider.Configs, metricClient, testLogger)
			go ps.Run()

			Eventually(promServer.requestHeaders).Should(Receive(And(
				HaveKeyWithValue("Header1", []string{"value1"}),
				HaveKeyWithValue("Header2", []string{"value2"}),
			)))
		})

		It("adds default tags if provided", func() {
			spyConfigProvider.scrapeConfigs = []scraper.PromScraperConfig{{
				Port:       promServer.port,
				SourceID:   "some-id",
				InstanceID: "some-instance-id",
				Labels: map[string]string{
					"default_label": "value",
				},
			}}
			promServer.resp = promOutput

			ps = app.NewPromScraper(cfg, spyConfigProvider.Configs, metricClient, testLogger)
			go ps.Run()

			Eventually(func() int {
				return len(spyAgent.Envelopes())
			}).Should(BeNumerically(">=", 5))

			for _, env := range spyAgent.Envelopes() {
				Expect(env.Tags).To(HaveKeyWithValue("default_label", "value"))
			}
		})

		Context("metrics path", func() {
			It("scrapes a different path if provided", func() {
				spyConfigProvider.scrapeConfigs = []scraper.PromScraperConfig{{
					Port:       promServer.port,
					SourceID:   "some-id",
					InstanceID: "some-instance-id",
					Path:       "/other/metrics/endpoint",
				}}

				promServer.resp = promOutput

				ps = app.NewPromScraper(cfg, spyConfigProvider.Configs, metricClient, testLogger)
				go ps.Run()

				Eventually(promServer.requestPaths).Should(Receive(Equal("/other/metrics/endpoint")))
			})
		})
	})

	Context("https", func() {
		BeforeEach(func() {
			promServer = newStubHttpsPromServer(testLogger, scrapeCerts, false)
			spyConfigProvider.scrapeConfigs = []scraper.PromScraperConfig{{
				Port:       promServer.port,
				SourceID:   "some-id",
				InstanceID: "some-instance-id",
				Scheme:     "https",
			}}

			promServer.resp = promOutput
		})

		It("scrapes https if provided", func() {
			ps = app.NewPromScraper(cfg, spyConfigProvider.Configs, metricClient, testLogger)
			go ps.Run()

			Eventually(spyAgent.Envelopes).Should(And(
				ContainElement(buildCounter("test_counter_prometheus_1", "some-id", "some-instance-id", 1)),
				ContainElement(buildGauge("test_gauge_prometheus_1", "some-id", "some-instance-id", 2)),
			))
		})

		It("respects skip SSL validation", func() {
			cfg.SkipSSLValidation = false
			ps = app.NewPromScraper(cfg, spyConfigProvider.Configs, metricClient, testLogger)
			go ps.Run()

			// certs have an untrusted CA
			Consistently(spyAgent.Envelopes, 1).Should(BeEmpty())
		})
	})

	Context("with TLS", func() {
		BeforeEach(func() {
			promServer = newStubHttpsPromServer(testLogger, scrapeCerts, true)
			promServer.resp = promOutput

			cfg.SkipSSLValidation = false
			cfg.ScrapeCertPath = scrapeCerts.Cert("client")
			cfg.ScrapeKeyPath = scrapeCerts.Key("client")
			cfg.ScrapeCACertPath = scrapeCerts.CA()
		})

		It("scrapes over mTLS", func() {
			spyConfigProvider.scrapeConfigs = []scraper.PromScraperConfig{{
				Port:       promServer.port,
				SourceID:   "some-id",
				InstanceID: "some-instance-id",
				Scheme:     "https",
				ServerName: "server",
			}}

			ps = app.NewPromScraper(cfg, spyConfigProvider.Configs, metricClient, testLogger)
			go ps.Run()

			Eventually(spyAgent.Envelopes).Should(And(
				ContainElement(buildCounter("test_counter_prometheus_1", "some-id", "some-instance-id", 1)),
				ContainElement(buildGauge("test_gauge_prometheus_1", "some-id", "some-instance-id", 2)),
			))
		})

		It("verifies server name if given", func() {
			spyConfigProvider.scrapeConfigs = []scraper.PromScraperConfig{{
				Port:       promServer.port,
				SourceID:   "some-id",
				InstanceID: "some-instance-id",
				Scheme:     "https",
				ServerName: "wrong",
			}}

			ps = app.NewPromScraper(cfg, spyConfigProvider.Configs, metricClient, testLogger)
			go ps.Run()

			Consistently(spyAgent.Envelopes, 1).Should(BeEmpty())
		})

		It("does not scrape if certs are provided but server name is empty", func() {
			spyConfigProvider.scrapeConfigs = []scraper.PromScraperConfig{{
				Port:       promServer.port,
				SourceID:   "some-id",
				InstanceID: "some-instance-id",
				Scheme:     "https",
			}}

			ps = app.NewPromScraper(cfg, spyConfigProvider.Configs, metricClient, testLogger)
			Expect(ps.Run).To(Panic())
		})
	})

	Context("with custom certs", func() {
		var customCerts = testhelper.GenerateCerts("custom")
		BeforeEach(func() {
			promServer = newStubHttpsPromServer(testLogger, customCerts, true)
			promServer.resp = promOutput

			cfg.SkipSSLValidation = false
			cfg.ScrapeCertPath = scrapeCerts.Cert("client")
			cfg.ScrapeKeyPath = scrapeCerts.Key("client")
			cfg.ScrapeCACertPath = scrapeCerts.CA()
		})

		It("scrapes over mTLS", func() {
			spyConfigProvider.scrapeConfigs = []scraper.PromScraperConfig{{
				Port:           promServer.port,
				SourceID:       "some-id",
				InstanceID:     "some-instance-id",
				Scheme:         "https",
				ServerName:     "server",
				CaPath:         customCerts.CA(),
				ClientKeyPath:  customCerts.Key("client"),
				ClientCertPath: customCerts.Cert("client"),
			}}

			ps = app.NewPromScraper(cfg, spyConfigProvider.Configs, metricClient, testLogger)
			go ps.Run()

			Eventually(spyAgent.Envelopes).Should(And(
				ContainElement(buildCounter("test_counter_prometheus_1", "some-id", "some-instance-id", 1)),
				ContainElement(buildGauge("test_gauge_prometheus_1", "some-id", "some-instance-id", 2)),
			))
		})
	})

	Context("metrics", func() {
		It("has scrape targets counter", func() {
			promServer = newStubPromServer()
			promServer2 := newStubPromServer()
			spyConfigProvider.scrapeConfigs = []scraper.PromScraperConfig{
				{
					Port:       promServer.port,
					SourceID:   "some-id",
					InstanceID: "some-instance-id",
					Path:       "metrics",
				},
				{
					Port:       promServer2.port,
					SourceID:   "some-id",
					InstanceID: "some-instance-id",
					Path:       "metrics",
				},
			}
			promServer.resp = promOutput
			promServer2.resp = promOutput2

			cfg.ConfigGlobs = append(cfg.ConfigGlobs, fmt.Sprintf("%s/metric_port*", metricConfigDir))
			ps = app.NewPromScraper(cfg, spyConfigProvider.Configs, metricClient, testLogger)
			go ps.Run()

			Eventually(hasMetric(metricClient, "scrape_targets_total", map[string]string{})).Should(BeTrue())
			Eventually(func() float64 {
				return metricClient.GetMetric("scrape_targets_total", map[string]string{}).Value()
			}).Should(Equal(2.0))
		})

		It("has failed scrapes counter", func() {
			promServer = newStubHttpsPromServer(testLogger, scrapeCerts, true)
			promServer.resp = promOutput

			cfg.SkipSSLValidation = false
			cfg.ScrapeCertPath = scrapeCerts.Cert("client")
			cfg.ScrapeKeyPath = scrapeCerts.Key("client")
			cfg.ScrapeCACertPath = scrapeCerts.CA()

			spyConfigProvider.scrapeConfigs = []scraper.PromScraperConfig{{
				Port:       promServer.port,
				SourceID:   "some-id",
				InstanceID: "some-instance-id",
				Path:       "metrics",
				Scheme:     "https",
				ServerName: "wrong",
			}}

			ps = app.NewPromScraper(cfg, spyConfigProvider.Configs, metricClient, testLogger)
			go ps.Run()

			Eventually(hasMetric(metricClient, "failed_scrapes_total", map[string]string{"scrape_target_source_id": "some-id"})).Should(BeTrue())
			Eventually(func() float64 {
				return metricClient.GetMetric("failed_scrapes_total", map[string]string{"scrape_target_source_id": "some-id"}).Value()
			}).Should(BeNumerically(">=", 1))
		})

		It("logs on recovery", func() {
			buf := &spyBuffer{}
			promServer = newStubPromServer()

			By("start scraper")
			spyConfigProvider.scrapeConfigs = []scraper.PromScraperConfig{
				{
					Port:       promServer.port,
					SourceID:   "some-id",
					InstanceID: "some-instance-id",
					Path:       "metrics",
				},
			}

			testLogger.SetOutput(buf)
			ps = app.NewPromScraper(cfg, spyConfigProvider.Configs, metricClient, testLogger)
			go ps.Run()

			By("simulate failure")
			promServer.setStatusCode(http.StatusGatewayTimeout)
			Eventually(buf.Read).Should(ContainSubstring("failed to scrape"))

			By("simulate recovery")
			promServer.setStatusCode(http.StatusOK)
			Eventually(buf.Read).Should(ContainSubstring("has recovered"))
		})
	})
})

func hasMetric(metricClient *metricsHelpers.SpyMetricsRegistry, name string, tags map[string]string) func() bool {
	return func() bool {
		return metricClient.HasMetric(name, tags)
	}
}

type stubPromServer struct {
	resp       string
	port       string
	statusCode int

	mu sync.Mutex

	requestHeaders chan http.Header
	requestPaths   chan string
}

func newStubPromServer() *stubPromServer {
	s := &stubPromServer{
		requestHeaders: make(chan http.Header, 100),
		requestPaths:   make(chan string, 100),
		statusCode:     http.StatusOK,
	}

	server := httptest.NewServer(s)

	addr := server.URL
	tokens := strings.Split(addr, ":")
	s.port = tokens[len(tokens)-1]

	return s
}

func newStubHttpsPromServer(testLogger *log.Logger, scrapeCerts *testhelper.TestCerts, mTLS bool) *stubPromServer {
	s := &stubPromServer{
		requestHeaders: make(chan http.Header, 100),
		requestPaths:   make(chan string, 100),
		statusCode:     http.StatusOK,
	}

	var serverOpts []tlsconfig.ServerOption
	if mTLS == true {
		serverOpts = append(serverOpts, tlsconfig.WithClientAuthenticationFromFile(scrapeCerts.CA()))
	}
	serverConf, err := tlsconfig.Build(
		tlsconfig.WithIdentityFromFile(scrapeCerts.Cert("server"), scrapeCerts.Key("server")),
	).Server(serverOpts...)
	Expect(err).ToNot(HaveOccurred())

	server := httptest.NewUnstartedServer(s)
	server.TLS = serverConf
	server.Config.ErrorLog = testLogger
	server.StartTLS()

	addr := server.Listener.Addr().String()
	tokens := strings.Split(addr, ":")
	s.port = tokens[len(tokens)-1]

	return s
}

func (s *stubPromServer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	s.requestHeaders <- req.Header
	s.requestPaths <- req.URL.Path

	s.mu.Lock()
	w.WriteHeader(s.statusCode)
	s.mu.Unlock()
	_, err := w.Write([]byte(s.resp))
	Expect(err).ToNot(HaveOccurred())
}

func (s *stubPromServer) setStatusCode(sc int) {
	s.mu.Lock()
	s.statusCode = sc
	s.mu.Unlock()
}

func buildGauge(name, sourceID, instanceID string, value float64) *loggregator_v2.Envelope {
	return &loggregator_v2.Envelope{
		SourceId:   sourceID,
		InstanceId: instanceID,
		Message: &loggregator_v2.Envelope_Gauge{
			Gauge: &loggregator_v2.Gauge{
				Metrics: map[string]*loggregator_v2.GaugeValue{
					name: {Value: value},
				},
			},
		},
	}
}

func buildCounter(name, sourceID, instanceID string, value float64) *loggregator_v2.Envelope {
	return &loggregator_v2.Envelope{
		SourceId:   sourceID,
		InstanceId: instanceID,
		Message: &loggregator_v2.Envelope_Counter{
			Counter: &loggregator_v2.Counter{
				Name:  name,
				Total: uint64(value),
			},
		},
	}
}

const (
	promOutput = `
# HELP test_counter_prometheus_1 test counter
# TYPE test_counter_prometheus_1 counter
test_counter_prometheus_1 1
# HELP test_gauge_prometheus_1 test gauge
# TYPE test_gauge_prometheus_1 gauge
test_gauge_prometheus_1 2
`

	promOutput2 = `
# HELP test_counter_prometheus_2 A second counter from another metrics url
# TYPE test_counter_prometheus_2 counter
test_counter_prometheus_2 3
`
)

func metricPortConfigDir() string {
	dir, err := os.MkdirTemp("", "")
	if err != nil {
		log.Fatal(err)
	}

	return dir
}

type spyAgent struct {
	loggregator_v2.IngressServer

	mu        sync.Mutex
	envelopes []*loggregator_v2.Envelope
	addr      string
}

func newSpyAgent(testCerts *testhelper.TestCerts) *spyAgent {
	agent := &spyAgent{}

	serverCreds, err := plumbing.NewServerCredentials(
		testCerts.Cert("metron"),
		testCerts.Key("metron"),
		testCerts.CA(),
	)
	if err != nil {
		panic(err)
	}

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		panic(err)
	}

	agent.addr = lis.Addr().String()

	grpcServer := grpc.NewServer(grpc.Creds(serverCreds))
	loggregator_v2.RegisterIngressServer(grpcServer, agent)

	go grpcServer.Serve(lis) //nolint:errcheck

	return agent
}

func (s *spyAgent) BatchSender(srv loggregator_v2.Ingress_BatchSenderServer) error {
	for {
		batch, err := srv.Recv()
		if err != nil {
			return err
		}

		for _, e := range batch.GetBatch() {
			if e.GetTimestamp() == 0 {
				panic("0 timestamp!?")
			}

			// We want to make our lives easier for matching against envelopes
			e.Timestamp = 0
		}

		s.mu.Lock()
		s.envelopes = append(s.envelopes, batch.GetBatch()...)
		s.mu.Unlock()
	}
}

func (s *spyAgent) Envelopes() []*loggregator_v2.Envelope {
	s.mu.Lock()
	defer s.mu.Unlock()

	results := make([]*loggregator_v2.Envelope, len(s.envelopes))
	copy(results, s.envelopes)
	return results
}

type spyConfigProvider struct {
	scrapeConfigs []scraper.PromScraperConfig
}

func newSpyConfigProvider() *spyConfigProvider {
	return &spyConfigProvider{}
}

func (p *spyConfigProvider) Configs() ([]scraper.PromScraperConfig, error) {
	var configsWithDefaults []scraper.PromScraperConfig
	for _, cfg := range p.scrapeConfigs {
		if cfg.Scheme == "" {
			cfg.Scheme = "http"
		}
		if cfg.Path == "" {
			cfg.Path = "metrics"
		}
		if cfg.ScrapeInterval.Nanoseconds() == 0 {
			cfg.ScrapeInterval = 100 * time.Millisecond
		}
		configsWithDefaults = append(configsWithDefaults, cfg)
	}

	return configsWithDefaults, nil
}

type spyBuffer struct {
	buf []byte
	sync.Mutex
}

func (s *spyBuffer) Read() string {
	s.Lock()
	defer s.Unlock()
	return string(s.buf)
}

func (s *spyBuffer) Write(p []byte) (n int, err error) {
	s.Lock()
	defer s.Unlock()
	s.buf = append(s.buf, p...)
	return len(p), nil
}
