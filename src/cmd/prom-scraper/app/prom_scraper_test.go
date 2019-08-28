package app_test

import (
	"code.cloudfoundry.org/loggregator-agent/cmd/prom-scraper/app"
	"fmt"
	"github.com/onsi/gomega/gexec"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"time"

	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	"code.cloudfoundry.org/loggregator-agent/internal/testhelper"
	"code.cloudfoundry.org/loggregator-agent/pkg/plumbing"
	"code.cloudfoundry.org/tlsconfig"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
)

var _ = Describe("PromScraper", func() {
	var (
		spyAgent     *spyAgent
		cfg          app.Config
		promServer   *stubPromServer
		ps           *app.PromScraper
		metricClient *testhelper.SpyMetricClient

		testLogger  = log.New(GinkgoWriter, "", log.LstdFlags)
		testCerts   = testhelper.GenerateCerts("loggregatorCA")
		scrapeCerts = testhelper.GenerateCerts("scrapeCA")

		metricConfigDir string
	)

	BeforeEach(func() {
		metricConfigDir = metricPortConfigDir()

		spyAgent = newSpyAgent(testCerts)
		metricClient = testhelper.NewMetricClient()

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

	Context("http", func() {
		BeforeEach(func() {
			promServer = newStubPromServer()
		})

		It("scrapes a prometheus endpoint and sends those metrics to a loggregator agent", func() {
			writeScrapeConfig(metricConfigDir, fmt.Sprintf(metricConfigTemplate, promServer.port), "prom_scraper_config.yml")
			promServer.resp = promOutput

			ps = app.NewPromScraper(cfg, metricClient, testLogger)
			go ps.Run()

			Eventually(spyAgent.Envelopes).Should(And(
				ContainElement(buildCounter("node_timex_pps_calibration_total", "some-id", "some-instance-id", 1)),
				ContainElement(buildCounter("node_timex_pps_error_total", "some-id", "some-instance-id", 2)),
				ContainElement(buildGauge("node_timex_pps_frequency_hertz", "some-id", "some-instance-id", 3)),
				ContainElement(buildGauge("node_timex_pps_jitter_seconds", "some-id", "some-instance-id", 4)),
				ContainElement(buildCounter("node_timex_pps_jitter_total", "some-id", "some-instance-id", 5)),
			))
		})

		It("scrapes prometheus endpoints after the specified interval", func() {
			promServer2 := newStubPromServer()
			writeScrapeConfig(metricConfigDir, fmt.Sprintf(metricConfigTemplate, promServer.port), "metric_port.yml")
			writeScrapeConfig(metricConfigDir, fmt.Sprintf(metricConfigWithScrapeIntervalTemplate, promServer2.port, "100ms"), "prom_scraper_config.yml")

			promServer.resp = promOutput
			promServer2.resp = promOutput2

			cfg.ConfigGlobs = append(cfg.ConfigGlobs, fmt.Sprintf("%s/metric_port*", metricConfigDir))
			cfg.DefaultScrapeInterval = time.Hour
			ps = app.NewPromScraper(cfg, metricClient, testLogger)
			go ps.Run()

			Eventually(spyAgent.Envelopes).Should(
				ContainElement(buildCounter("node2_counter", "some-id", "some-instance-id", 6)),
			)

			Consistently(spyAgent.Envelopes).ShouldNot(
				ContainElement(buildCounter("node_timex_pps_calibration_total", "some-id", "some-instance-id", 1)),
			)
		})

		It("scrapes multiple prometheus endpoints", func() {
			promServer2 := newStubPromServer()
			writeScrapeConfig(metricConfigDir, fmt.Sprintf(metricConfigTemplate, promServer.port), "metric_port.yml")
			writeScrapeConfig(metricConfigDir, fmt.Sprintf(metricConfigTemplate, promServer2.port), "prom_scraper_config.yml")

			promServer.resp = promOutput
			promServer2.resp = promOutput2

			cfg.ConfigGlobs = append(cfg.ConfigGlobs, fmt.Sprintf("%s/metric_port*", metricConfigDir))
			ps = app.NewPromScraper(cfg, metricClient, testLogger)
			go ps.Run()

			Eventually(spyAgent.Envelopes).Should(And(
				ContainElement(buildCounter("node_timex_pps_calibration_total", "some-id", "some-instance-id", 1)),
				ContainElement(buildCounter("node_timex_pps_error_total", "some-id", "some-instance-id", 2)),
				ContainElement(buildGauge("node_timex_pps_frequency_hertz", "some-id", "some-instance-id", 3)),
				ContainElement(buildGauge("node_timex_pps_jitter_seconds", "some-id", "some-instance-id", 4)),
				ContainElement(buildCounter("node_timex_pps_jitter_total", "some-id", "some-instance-id", 5)),
				ContainElement(buildCounter("node2_counter", "some-id", "some-instance-id", 6)),
			))
		})

		It("scrapes with headers if provided", func() {
			writeScrapeConfig(metricConfigDir, fmt.Sprintf(metricConfigWithHeadersTemplate, promServer.port), "prom_scraper_config.yml")
			promServer.resp = promOutput

			ps = app.NewPromScraper(cfg, metricClient, testLogger)
			go ps.Run()

			Eventually(promServer.requestHeaders).Should(Receive(And(
				HaveKeyWithValue("Header1", []string{"value1"}),
				HaveKeyWithValue("Header2", []string{"value2"}),
			)))
		})

		It("adds default tags if provided", func() {
			writeScrapeConfig(
				metricConfigDir,
				fmt.Sprintf(metricConfigWithLabelsTemplate, promServer.port, `default_label: "value"`),
				"prom_scraper_config.yml",
			)
			promServer.resp = promOutput

			ps = app.NewPromScraper(cfg, metricClient, testLogger)
			go ps.Run()

			Eventually(func() int {
				return len(spyAgent.Envelopes())
			}).Should(BeNumerically(">=", 5))

			for _, env := range spyAgent.Envelopes() {
				Expect(env.Tags).To(HaveKeyWithValue("default_label", "value"))
			}
		})

		Context("metrics path", func() {
			It("defaults to /metrics", func() {
				writeScrapeConfig(metricConfigDir, fmt.Sprintf(metricConfigTemplate, promServer.port), "prom_scraper_config.yml")
				promServer.resp = promOutput

				ps = app.NewPromScraper(cfg, metricClient, testLogger)
				go ps.Run()

				Eventually(promServer.requestPaths).Should(Receive(Equal("/metrics")))
			})

			It("scrapes a different path if provided", func() {
				writeScrapeConfig(
					metricConfigDir,
					fmt.Sprintf(metricConfigWithPathTemplate, promServer.port, "/other/metrics/endpoint"),
					"prom_scraper_config.yml",
				)

				writeScrapeConfig(metricConfigDir, fmt.Sprintf(metricConfigWithHeadersTemplate, promServer.port), "prom_scraper_config.yml")
				promServer.resp = promOutput

				ps = app.NewPromScraper(cfg, metricClient, testLogger)
				go ps.Run()

				Eventually(promServer.requestPaths).Should(Receive(Equal("/other/metrics/endpoint")))
			})
		})
	})

	Context("https", func() {
		BeforeEach(func() {
			promServer = newStubHttpsPromServer(testLogger, scrapeCerts, false)
			writeScrapeConfig(
				metricConfigDir,
				fmt.Sprintf(metricConfigWithSchemeTemplate, promServer.port, ""),
				"prom_scraper_config.yml",
			)

			promServer.resp = promOutput
		})

		It("scrapes https if provided", func() {
			ps = app.NewPromScraper(cfg, metricClient, testLogger)
			go ps.Run()

			Eventually(spyAgent.Envelopes).Should(And(
				ContainElement(buildCounter("node_timex_pps_calibration_total", "some-id", "some-instance-id", 1)),
				ContainElement(buildCounter("node_timex_pps_error_total", "some-id", "some-instance-id", 2)),
				ContainElement(buildGauge("node_timex_pps_frequency_hertz", "some-id", "some-instance-id", 3)),
				ContainElement(buildGauge("node_timex_pps_jitter_seconds", "some-id", "some-instance-id", 4)),
				ContainElement(buildCounter("node_timex_pps_jitter_total", "some-id", "some-instance-id", 5)),
			))
		})

		It("respects skip SSL validation", func() {
			cfg.SkipSSLValidation = false
			ps = app.NewPromScraper(cfg, metricClient, testLogger)
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
			writeScrapeConfig(
				metricConfigDir,
				fmt.Sprintf(metricConfigWithSchemeTemplate, promServer.port, "server_name: server"),
				"prom_scraper_config.yml",
			)

			ps = app.NewPromScraper(cfg, metricClient, testLogger)
			go ps.Run()

			Eventually(spyAgent.Envelopes).Should(And(
				ContainElement(buildCounter("node_timex_pps_calibration_total", "some-id", "some-instance-id", 1)),
				ContainElement(buildCounter("node_timex_pps_error_total", "some-id", "some-instance-id", 2)),
				ContainElement(buildGauge("node_timex_pps_frequency_hertz", "some-id", "some-instance-id", 3)),
				ContainElement(buildGauge("node_timex_pps_jitter_seconds", "some-id", "some-instance-id", 4)),
				ContainElement(buildCounter("node_timex_pps_jitter_total", "some-id", "some-instance-id", 5)),
			))
		})

		It("verifies server name if given", func() {
			writeScrapeConfig(
				metricConfigDir,
				fmt.Sprintf(metricConfigWithSchemeTemplate, promServer.port, "server_name: wrong"),
				"prom_scraper_config.yml",
			)

			ps = app.NewPromScraper(cfg, metricClient, testLogger)
			go ps.Run()

			Consistently(spyAgent.Envelopes, 1).Should(BeEmpty())
		})

		It("does not scrape if certs are provided but server name is empty", func() {
			writeScrapeConfig(
				metricConfigDir,
				fmt.Sprintf(metricConfigWithSchemeTemplate, promServer.port, ""),
				"prom_scraper_config.yml",
			)

			ps = app.NewPromScraper(cfg, metricClient, testLogger)
			Expect(ps.Run).To(Panic())
		})
	})

	Context("metrics", func() {
		It("has scrape targets counter", func() {
			promServer = newStubPromServer()
			promServer2 := newStubPromServer()
			writeScrapeConfig(metricConfigDir, fmt.Sprintf(metricConfigTemplate, promServer.port), "metric_port.yml")
			writeScrapeConfig(metricConfigDir, fmt.Sprintf(metricConfigTemplate, promServer2.port), "prom_scraper_config.yml")

			promServer.resp = promOutput
			promServer2.resp = promOutput2

			cfg.ConfigGlobs = append(cfg.ConfigGlobs, fmt.Sprintf("%s/metric_port*", metricConfigDir))
			ps = app.NewPromScraper(cfg, metricClient, testLogger)
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

			writeScrapeConfig(
				metricConfigDir,
				fmt.Sprintf(metricConfigWithSchemeTemplate, promServer.port, "server_name: wrong"),
				"prom_scraper_config.yml",
			)

			ps = app.NewPromScraper(cfg, metricClient, testLogger)
			go ps.Run()

			Eventually(hasMetric(metricClient, "failed_scrapes_total", map[string]string{"scrape_target_source_id": "some-id"})).Should(BeTrue())
			Eventually(func() float64 {
				return metricClient.GetMetric("failed_scrapes_total", map[string]string{"scrape_target_source_id": "some-id"}).Value()
			}).Should(BeNumerically(">=", 1))
		})
	})
})

func hasMetric(metricClient *testhelper.SpyMetricClient, name string, tags map[string]string) func() bool {
	return func() bool {
		return metricClient.HasMetric(name, tags)
	}
}

type stubPromServer struct {
	resp string
	port string

	requestHeaders chan http.Header
	requestPaths   chan string
}

func newStubPromServer() *stubPromServer {
	s := &stubPromServer{
		requestHeaders: make(chan http.Header, 100),
		requestPaths:   make(chan string, 100),
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
	w.Write([]byte(s.resp))
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
# HELP node_timex_pps_calibration_total Pulse per second count of calibration intervals.
# TYPE node_timex_pps_calibration_total counter
node_timex_pps_calibration_total 1
# HELP node_timex_pps_error_total Pulse per second count of calibration errors.
# TYPE node_timex_pps_error_total counter
node_timex_pps_error_total 2
# HELP node_timex_pps_frequency_hertz Pulse per second frequency.
# TYPE node_timex_pps_frequency_hertz gauge
node_timex_pps_frequency_hertz 3
# HELP node_timex_pps_jitter_seconds Pulse per second jitter.
# TYPE node_timex_pps_jitter_seconds gauge
node_timex_pps_jitter_seconds 4
# HELP node_timex_pps_jitter_total Pulse per second count of jitter limit exceeded events.
# TYPE node_timex_pps_jitter_total counter
node_timex_pps_jitter_total 5
`

	promOutput2 = `
# HELP node2_counter A second counter from another metrics url
# TYPE node2_counter counter
node2_counter 6
`

	metricConfigTemplate = `---
port: %s
source_id: some-id
instance_id: some-instance-id`

	metricConfigWithScrapeIntervalTemplate = `---
port: %s
source_id: some-id
instance_id: some-instance-id
scrape_interval: %s`

	metricConfigWithPathTemplate = `---
port: %s
source_id: some-id
instance_id: some-instance-id
path: %s`

	metricConfigWithSchemeTemplate = `---
port: %s
source_id: some-id
instance_id: some-instance-id
scheme: https
%s`

	metricConfigWithHeadersTemplate = `---
port: %s
source_id: some-id
instance_id: some-instance-id
headers:
  Header1: value1
  Header2: value2`

	metricConfigWithLabelsTemplate = `---
port: %s
source_id: some-id
instance_id: some-instance-id
labels:
  %s`
)

func metricPortConfigDir() string {
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		log.Fatal(err)
	}

	return dir
}

func writeScrapeConfig(metricConfigDir, config, fileName string) {
	f, err := ioutil.TempFile(metricConfigDir, fileName)
	if err != nil {
		log.Fatal(err)
	}

	_, err = f.Write([]byte(config))
	if err != nil {
		log.Fatal(err)
	}
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

	go grpcServer.Serve(lis)

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
