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
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
)

var _ = Describe("PromScraper", func() {
	var (
		spyAgent   *spyAgent
		cfg        app.Config
		promServer *stubPromServer

		testLogger = log.New(GinkgoWriter, "", log.LstdFlags)

		metricConfigDir string
	)

	BeforeEach(func() {
		promServer = newStubPromServer()
		metricConfigDir = metricPortConfigDir()

		spyAgent = newSpyAgent()

		cfg = app.Config{
			ClientKeyPath:          testhelper.Cert("metron.key"),
			ClientCertPath:         testhelper.Cert("metron.crt"),
			CACertPath:             testhelper.Cert("loggregator-ca.crt"),
			LoggregatorIngressAddr: spyAgent.addr,
			MetricPortCfg:          fmt.Sprintf("%s/*", metricConfigDir),
			ScrapeInterval:         100 * time.Millisecond,
		}
	})

	AfterEach(func() {
		os.RemoveAll(metricConfigDir)
		gexec.CleanupBuildArtifacts()
	})

	It("scrapes a prometheus endpoint and sends those metrics to a loggregator agent", func() {
		writeScrapeConfig(metricConfigDir, fmt.Sprintf(metricConfigTemplate, promServer.port))
		promServer.resp = promOutput

		ps := app.NewPromScraper(cfg, testLogger)
		go ps.Run()

		Eventually(spyAgent.Envelopes).Should(And(
			ContainElement(buildCounter("node_timex_pps_calibration_total", "some-id", "some-instance-id", 1)),
			ContainElement(buildCounter("node_timex_pps_error_total", "some-id", "some-instance-id", 2)),
			ContainElement(buildGauge("node_timex_pps_frequency_hertz", "some-id", "some-instance-id", 3)),
			ContainElement(buildGauge("node_timex_pps_jitter_seconds", "some-id", "some-instance-id", 4)),
			ContainElement(buildCounter("node_timex_pps_jitter_total", "some-id", "some-instance-id", 5)),
		))
	})

	It("scrapes multiple prometheus endpoints and sends those metrics to a loggregator agent", func() {
		promServer2 := newStubPromServer()
		writeScrapeConfig(metricConfigDir, fmt.Sprintf(metricConfigTemplate, promServer.port))
		writeScrapeConfig(metricConfigDir, fmt.Sprintf(metricConfigTemplate, promServer2.port))

		promServer.resp = promOutput
		promServer2.resp = promOutput2

		ps := app.NewPromScraper(cfg, testLogger)
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
		writeScrapeConfig(metricConfigDir, fmt.Sprintf(metricConfigWithHeadersTemplate, promServer.port))
		promServer.resp = promOutput

		ps := app.NewPromScraper(cfg, testLogger)
		go ps.Run()

		Eventually(promServer.requestHeaders).Should(Receive(And(
			HaveKeyWithValue("Header1", []string{"value1"}),
			HaveKeyWithValue("Header2", []string{"value2"}),
		)))
	})
})

func newStubPromServer() *stubPromServer {
	s := &stubPromServer{}

	server := httptest.NewServer(s)

	addr := server.Listener.Addr().String()
	tokens := strings.Split(addr, ":")
	s.port = tokens[len(tokens)-1]
	s.requestHeaders = make(chan http.Header, 100)

	return s
}

type stubPromServer struct {
	resp string
	port string

	requestHeaders chan http.Header
}

func (s *stubPromServer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	s.requestHeaders <- req.Header
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
)

const (
	promOutput2 = `
# HELP node2_counter A second counter from another metrics url
# TYPE node2_counter counter
node2_counter 6
`
)

const metricConfigTemplate = `---
port: %s
source_id: some-id
instance_id: some-instance-id`

const metricConfigWithHeadersTemplate = `---
port: %s
source_id: some-id
instance_id: some-instance-id
headers:
  Header1: value1
  Header2: value2`

func metricPortConfigDir() string {
	dir, err := ioutil.TempDir(".", "")
	if err != nil {
		log.Fatal(err)
	}

	return dir
}

func writeScrapeConfig(metricConfigDir, config string) {
	f, err := ioutil.TempFile(metricConfigDir, "metric_port.yml")
	Expect(err).ToNot(HaveOccurred())

	_, err = f.Write([]byte(config))
	Expect(err).ToNot(HaveOccurred())
}

type spyAgent struct {
	loggregator_v2.IngressServer

	mu        sync.Mutex
	envelopes []*loggregator_v2.Envelope
	addr      string
}

func newSpyAgent() *spyAgent {
	agent := &spyAgent{}

	serverCreds, err := plumbing.NewServerCredentials(
		testhelper.Cert("metron.crt"),
		testhelper.Cert("metron.key"),
		testhelper.Cert("loggregator-ca.crt"),
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
