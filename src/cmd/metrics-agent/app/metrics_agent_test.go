package app_test

import (
	"code.cloudfoundry.org/go-loggregator"
	"code.cloudfoundry.org/loggregator-agent/cmd/metrics-agent/app"
	"code.cloudfoundry.org/loggregator-agent/internal/testhelper"
	"code.cloudfoundry.org/tlsconfig"
	"context"
	"fmt"
	"github.com/gogo/protobuf/proto"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

var _ = Describe("MetricsAgent", func() {
	var (
		metricsAgent *app.MetricsAgent
		grpcPort     uint16
		metricsPort  uint16
		testCerts    *testhelper.TestCerts

		ingressClient *loggregator.IngressClient
	)

	BeforeEach(func() {
		testCerts = testhelper.GenerateCerts("loggregatorCA")

		grpcPort = getFreePort()
		metricsPort = getFreePort()
		cfg := app.Config{
			Metrics: app.MetricsConfig{
				Port:     metricsPort,
				CAFile:   testCerts.CA(),
				CertFile: testCerts.Cert("client"),
				KeyFile:  testCerts.Key("client"),
			},
			Tags: map[string]string{
				"a": "1",
				"b": "2",
			},
			GRPC: app.GRPCConfig{
				Port:     grpcPort,
				CAFile:   testCerts.CA(),
				CertFile: testCerts.Cert("metron"),
				KeyFile:  testCerts.Key("metron"),
			},
		}

		ingressClient = newTestingIngressClient(int(grpcPort), testCerts)

		testLogger := log.New(GinkgoWriter, "", log.LstdFlags)
		metricsAgent = app.NewMetricsAgent(cfg, testhelper.NewMetricClient(), testLogger)
		go metricsAgent.Run()
		waitForMetricsEndpoint(metricsPort, testCerts)
	})

	AfterEach(func() {
		metricsAgent.Stop()
	})

	It("exposes counters on a prometheus endpoint", func() {
		cancel := doUntilCancelled(func() {
			ingressClient.EmitCounter("total_counter", loggregator.WithTotal(22))
		})
		defer cancel()

		Eventually(getMetricFamilies(metricsPort, testCerts), 3).Should(HaveKey("total_counter"))

		metric := getMetric("total_counter", metricsPort, testCerts)
		Expect(metric.GetCounter().GetValue()).To(BeNumerically("==", 22))
	})

	It("includes default tags", func() {
		cancel := doUntilCancelled(func() {
			ingressClient.EmitCounter("total_counter",
				loggregator.WithTotal(22),
				loggregator.WithCounterSourceInfo("some-source-id", "some-instance-id"),
			)
		})
		defer cancel()

		Eventually(getMetricFamilies(metricsPort, testCerts), 3).Should(HaveKey("total_counter"))

		metric := getMetric("total_counter", metricsPort, testCerts)
		Expect(metric.GetLabel()).To(ConsistOf(
			&dto.LabelPair{Name: proto.String("a"), Value: proto.String("1")},
			&dto.LabelPair{Name: proto.String("b"), Value: proto.String("2")},
			&dto.LabelPair{Name: proto.String("source_id"), Value: proto.String("some-source-id")},
			&dto.LabelPair{Name: proto.String("instance_id"), Value: proto.String("some-instance-id")},
		))
	})

	It("aggregates delta counters", func() {
		cancel := doUntilCancelled(func() {
			ingressClient.EmitCounter("delta_counter", loggregator.WithDelta(2))
		})
		defer cancel()

		Eventually(getMetricFamilies(metricsPort, testCerts), 3).Should(HaveKey("delta_counter"))

		originialValue := getMetric("delta_counter", metricsPort, testCerts).GetCounter().GetValue()

		Eventually(func() float64 {
			metric := getMetric("delta_counter", metricsPort, testCerts)
			if metric == nil {
				return 0
			}
			return metric.GetCounter().GetValue()
		}).Should(BeNumerically(">", originialValue))
	})
})

func doUntilCancelled(f func()) context.CancelFunc {
	ctx, cancelFunc := context.WithCancel(context.Background())
	wg := sync.WaitGroup{}

	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.Tick(100 * time.Millisecond)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker:
				f()
			}
		}
	}()

	return cancelFunc
}

func waitForMetricsEndpoint(port uint16, testCerts *testhelper.TestCerts) {
	Eventually(func() error {
		_, err := getMetricsResponse(port, testCerts)
		return err
	}).Should(Succeed())
}

func getMetricsResponse(port uint16, testCerts *testhelper.TestCerts) (*http.Response, error) {
	tlsConfig, err := tlsconfig.Build(tlsconfig.WithIdentityFromFile(testCerts.Cert("client"), testCerts.Key("client"))).
		Client(tlsconfig.WithAuthorityFromFile(testCerts.CA()))
	if err != nil {
		return nil, err
	}

	client := &http.Client{
		Transport: &http.Transport{TLSClientConfig: tlsConfig},
	}

	url := fmt.Sprintf("https://127.0.0.1:%d/metrics", port)
	resp, err := client.Get(url)
	if err == nil && resp.StatusCode != http.StatusOK {
		return resp, fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}

	return resp, err
}

func getMetricFamilies(port uint16, testCerts *testhelper.TestCerts) func() map[string]*dto.MetricFamily {
	return func() map[string]*dto.MetricFamily {
		resp, err := getMetricsResponse(port, testCerts)

		metricFamilies, err := new(expfmt.TextParser).TextToMetricFamilies(resp.Body)
		if err != nil {
			return nil
		}

		return metricFamilies
	}
}

func getMetric(metricName string, port uint16, testCerts *testhelper.TestCerts) *dto.Metric {
	families := getMetricFamilies(port, testCerts)()
	family, ok := families[metricName]
	if !ok {
		return nil
	}

	metrics := family.Metric
	Expect(metrics).To(HaveLen(1))
	return metrics[0]
}

func newTestingIngressClient(grpcPort int, testCerts *testhelper.TestCerts) *loggregator.IngressClient {
	tlsConfig, err := loggregator.NewIngressTLSConfig(testCerts.CA(), testCerts.Cert("metron"), testCerts.Key("metron"))
	Expect(err).ToNot(HaveOccurred())

	ingressClient, err := loggregator.NewIngressClient(
		tlsConfig,
		loggregator.WithAddr(fmt.Sprintf("127.0.0.1:%d", grpcPort)),
		loggregator.WithLogger(log.New(GinkgoWriter, "[TEST INGRESS CLIENT] ", 0)),
		loggregator.WithBatchMaxSize(1),
	)
	Expect(err).ToNot(HaveOccurred())

	return ingressClient
}

func getFreePort() uint16 {
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		log.Fatal(err)
	}

	defer l.Close()
	return uint16(l.Addr().(*net.TCPAddr).Port)
}
