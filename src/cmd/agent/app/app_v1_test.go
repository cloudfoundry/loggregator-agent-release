package app_test

import (
	"code.cloudfoundry.org/loggregator-agent-release/src/internal/testhelper"
	"fmt"
	"net"
	"sync"

	metricHelpers "code.cloudfoundry.org/go-metric-registry/testhelpers"
	"code.cloudfoundry.org/loggregator-agent-release/src/cmd/agent/app"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/plumbing"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("v1 App", func() {
	var testCerts = testhelper.GenerateCerts("loggregatorCA")

	It("uses DopplerAddrWithAZ for AZ affinity", func() {
		spyLookup := newSpyLookup()

		clientCreds, err := plumbing.NewClientCredentials(
			testCerts.Cert("metron"),
			testCerts.Key("metron"),
			testCerts.CA(),
			"doppler",
		)
		Expect(err).ToNot(HaveOccurred())

		config := buildAgentConfig("127.0.0.1", 1234)
		config.Zone = "something-bad"
		expectedHost, _, err := net.SplitHostPort(config.RouterAddrWithAZ)
		Expect(err).ToNot(HaveOccurred())

		app := app.NewV1App(
			&config,
			clientCreds,
			metricHelpers.NewMetricsRegistry(),
			app.WithV1Lookup(spyLookup.lookup),
		)
		go app.Start()

		Eventually(spyLookup.calledWith(expectedHost)).Should(BeTrue())
	})

	It("emits the expected V1 metrics", func() {
		spyLookup := newSpyLookup()

		clientCreds, err := plumbing.NewClientCredentials(
			testCerts.Cert("metron"),
			testCerts.Key("metron"),
			testCerts.CA(),
			"doppler",
		)
		Expect(err).ToNot(HaveOccurred())

		config := buildAgentConfig("127.0.0.1", 1234)
		config.Zone = "something-bad"
		Expect(err).ToNot(HaveOccurred())

		mc := metricHelpers.NewMetricsRegistry()

		app := app.NewV1App(
			&config,
			clientCreds,
			mc,
			app.WithV1Lookup(spyLookup.lookup),
		)
		go app.Start()

		Eventually(hasMetric(mc, "ingress", map[string]string{"metric_version": "1.0"})).Should(BeTrue())
		Eventually(hasMetric(mc, "egress", map[string]string{"metric_version": "1.0"})).Should(BeTrue())
		Eventually(hasMetric(mc, "dropped", map[string]string{"direction": "all", "metric_version": "1.0"})).Should(BeTrue())
		Eventually(hasMetric(mc, "average_envelopes", map[string]string{"unit": "bytes/minute", "metric_version": "1.0", "loggregator": "v1"})).Should(BeTrue())
	})
})

func hasMetric(mc *metricHelpers.SpyMetricsRegistry, metricName string, tags map[string]string) func() bool {
	return func() bool {
		return mc.HasMetric(metricName, tags)
	}
}

type spyLookup struct {
	mu          sync.Mutex
	_calledWith map[string]struct{}
}

func newSpyLookup() *spyLookup {
	return &spyLookup{
		_calledWith: make(map[string]struct{}),
	}
}

func (s *spyLookup) calledWith(host string) func() bool {
	return func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		_, ok := s._calledWith[host]
		return ok
	}
}

func (s *spyLookup) lookup(host string) ([]net.IP, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s._calledWith[host] = struct{}{}
	return []net.IP{
		net.IPv4(byte(127), byte(0), byte(0), byte(1)),
	}, nil
}

func buildAgentConfig(dopplerURI string, dopplerGRPCPort int) app.Config {
	availabilityZone := "test-availability-zone"
	jobName := "test-job-name"
	jobIndex := "42"

	testCerts := testhelper.GenerateCerts("loggregatorCA")

	return app.Config{
		Index: jobIndex,
		Job:   jobName,
		Zone:  availabilityZone,

		Tags: map[string]string{
			"auto-tag-1": "auto-tag-value-1",
			"auto-tag-2": "auto-tag-value-2",
		},

		Deployment: "deployment",

		RouterAddr:       fmt.Sprintf("%s:%d", dopplerURI, dopplerGRPCPort),
		RouterAddrWithAZ: fmt.Sprintf("%s.%s:%d", availabilityZone, dopplerURI, dopplerGRPCPort),

		GRPC: app.GRPC{
			CertFile: testCerts.Cert("metron"),
			KeyFile:  testCerts.Key("metron"),
			CAFile:   testCerts.CA(),
		},

		MetricBatchIntervalMilliseconds: 5000,
	}
}
