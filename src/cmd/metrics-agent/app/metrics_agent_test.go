package app_test

import (
	"code.cloudfoundry.org/go-loggregator"
	"code.cloudfoundry.org/loggregator-agent/cmd/metrics-agent/app"
	"code.cloudfoundry.org/loggregator-agent/internal/testhelper"
	"code.cloudfoundry.org/tlsconfig"
	"code.cloudfoundry.org/tlsconfig/certtest"
	"context"
	"fmt"
	"github.com/gogo/protobuf/proto"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"time"
)

var _ = Describe("MetricsAgent", func() {
	var (
		cfg          app.Config
		metricsAgent *app.MetricsAgent
		grpcPort     uint16
		metricsPort  uint16
		certDir      string

		ingressClient *loggregator.IngressClient
	)

	BeforeEach(func() {
		certDir = generateCerts("loggregatorCA", "metron", "client")

		grpcPort = getFreePort()
		metricsPort = getFreePort()
		cfg = app.Config{
			Metrics: app.MetricsConfig{
				Port:     metricsPort,
				CAFile:   certDir + "/loggregatorCA.cert",
				CertFile: certDir + "/client.cert",
				KeyFile:  certDir + "/client.key",
			},
			Tags: map[string]string{
				"a": "1",
				"b": "2",
			},
			GRPC: app.GRPCConfig{
				Port:     grpcPort,
				CAFile:   certDir + "/loggregatorCA.cert",
				CertFile: certDir + "/metron.cert",
				KeyFile:  certDir + "/metron.key",
			},
		}

		ingressClient = newTestingIngressClient(int(grpcPort), certDir+"/loggregatorCA.cert", certDir+"/metron.cert", certDir+"/metron.key")

		testLogger := log.New(GinkgoWriter, "", log.LstdFlags)
		metricsAgent = app.NewMetricsAgent(cfg, testhelper.NewMetricClient(), testLogger)
		go metricsAgent.Run()
		waitForMetricsEndpoint(metricsPort, certDir)
	})

	AfterEach(func() {
		metricsAgent.Stop()
	})

	It("exposes counters on a prometheus endpoint", func() {
		cancel := doUntilCancelled(func() {
			ingressClient.EmitCounter("total_counter", loggregator.WithTotal(22))
		})
		defer cancel()

		Eventually(getMetricFamilies(metricsPort, certDir), 3).Should(HaveKey("total_counter"))

		metric := getMetric("total_counter", metricsPort, certDir)
		Expect(metric.GetCounter().GetValue()).To(BeNumerically("==", 22))
	})

	It("includes default tags", func() {
		cancel := doUntilCancelled(func() {
			ingressClient.EmitCounter("total_counter", loggregator.WithTotal(22))
		})
		defer cancel()

		Eventually(getMetricFamilies(metricsPort, certDir), 3).Should(HaveKey("total_counter"))

		metric := getMetric("total_counter", metricsPort, certDir)
		Expect(metric.GetLabel()).To(ConsistOf(
			&dto.LabelPair{Name: proto.String("a"), Value: proto.String("1")},
			&dto.LabelPair{Name: proto.String("b"), Value: proto.String("2")},
		))
	})

	It("aggregates delta counters", func() {
		cancel := doUntilCancelled(func() {
			ingressClient.EmitCounter("delta_counter", loggregator.WithDelta(2))
		})
		defer cancel()

		Eventually(getMetricFamilies(metricsPort, certDir), 3).Should(HaveKey("delta_counter"))

		originialValue := getMetric("delta_counter", metricsPort, certDir).GetCounter().GetValue()

		Eventually(func() float64 {
			metric := getMetric("delta_counter", metricsPort, certDir)
			if metric == nil {
				return 0
			}
			return metric.GetCounter().GetValue()
		}).Should(BeNumerically(">", originialValue))
	})
})

func doUntilCancelled(f func()) context.CancelFunc {
	ctx, cancelFunc := context.WithCancel(context.Background())

	go func() {
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

func waitForMetricsEndpoint(port uint16, certDir string) {
	Eventually(func() error {
		_, err := getMetricsResponse(port, certDir)
		return err
	}).Should(Succeed())
}

func getMetricsResponse(port uint16, certDir string) (*http.Response, error) {
	tlsConfig, err := tlsconfig.Build(tlsconfig.WithIdentityFromFile(certDir+"/client.cert", certDir+"/client.key")).Client(
		tlsconfig.WithAuthorityFromFile(certDir + "/loggregatorCA.cert"))
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

func getMetricFamilies(port uint16, certDir string) func() map[string]*dto.MetricFamily {
	return func() map[string]*dto.MetricFamily {
		resp, err := getMetricsResponse(port, certDir)

		metricFamilies, err := new(expfmt.TextParser).TextToMetricFamilies(resp.Body)
		if err != nil {
			return nil
		}

		return metricFamilies
	}
}

func getMetric(metricName string, port uint16, certDir string) *dto.Metric {
	families := getMetricFamilies(port, certDir)()
	family, ok := families[metricName]
	if !ok {
		return nil
	}

	metrics := family.Metric
	Expect(metrics).To(HaveLen(1))
	return metrics[0]
}

func newTestingIngressClient(grpcPort int, ca, cert, key string) *loggregator.IngressClient {
	tlsConfig, err := loggregator.NewIngressTLSConfig(ca, cert, key)
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

func generateCerts(caName string, commonNames ...string) string {
	tmpDir, err := ioutil.TempDir("", caName)
	Expect(err).ToNot(HaveOccurred())

	ca, err := certtest.BuildCA(caName)
	Expect(err).ToNot(HaveOccurred())

	caBytes, err := ca.CertificatePEM()
	Expect(err).ToNot(HaveOccurred())

	err = ioutil.WriteFile(fmt.Sprintf("%s/%s.cert", tmpDir, caName), caBytes, 0600)

	for _, commonName := range commonNames {
		cert, err := ca.BuildSignedCertificate(commonName, certtest.WithDomains(commonName))
		Expect(err).ToNot(HaveOccurred())

		certBytes, keyBytes, err := cert.CertificatePEMAndPrivateKey()
		Expect(err).ToNot(HaveOccurred())

		err = ioutil.WriteFile(fmt.Sprintf("%s/%s.cert", tmpDir, commonName), certBytes, 0600)
		Expect(err).ToNot(HaveOccurred())
		err = ioutil.WriteFile(fmt.Sprintf("%s/%s.key", tmpDir, commonName), keyBytes, 0600)
		Expect(err).ToNot(HaveOccurred())
	}

	return tmpDir
}

func writeToTempFile(tmpDir, pattern string, contents []byte) {
	file, err := ioutil.TempFile(tmpDir, pattern)
	Expect(err).ToNot(HaveOccurred())

	_, err = file.Write(contents)
	Expect(err).ToNot(HaveOccurred())

	Expect(file.Close()).To(Succeed())

	println(file.Name())
}

func getFreePort() uint16 {
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		log.Fatal(err)
	}

	defer l.Close()
	return uint16(l.Addr().(*net.TCPAddr).Port)
}
