package app_test

import (
	"net"

	"code.cloudfoundry.org/loggregator-agent/cmd/agent/app"
	"code.cloudfoundry.org/loggregator-agent/internal/testhelper"
	"code.cloudfoundry.org/loggregator-agent/pkg/plumbing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("v2 App", func() {
	It("uses DopplerAddrWithAZ for AZ affinity", func() {
		spyLookup := newSpyLookup()

		clientCreds, err := plumbing.NewClientCredentials(
			testhelper.Cert("metron.crt"),
			testhelper.Cert("metron.key"),
			testhelper.Cert("loggregator-ca.crt"),
			"doppler",
		)
		Expect(err).ToNot(HaveOccurred())

		serverCreds, err := plumbing.NewServerCredentials(
			testhelper.Cert("router.crt"),
			testhelper.Cert("router.key"),
			testhelper.Cert("loggregator-ca.crt"),
		)
		Expect(err).ToNot(HaveOccurred())

		config := buildAgentConfig("127.0.0.1", 1234)
		config.Zone = "something-bad"
		expectedHost, _, err := net.SplitHostPort(config.RouterAddrWithAZ)
		Expect(err).ToNot(HaveOccurred())

		app := app.NewV2App(
			&config,
			clientCreds,
			serverCreds,
			testhelper.NewMetricClient(),
			app.WithV2Lookup(spyLookup.lookup),
		)
		go app.Start()

		Eventually(spyLookup.calledWith(expectedHost)).Should(BeTrue())
	})

	It("emits v2 metrics", func() {
		spyLookup := newSpyLookup()

		clientCreds, err := plumbing.NewClientCredentials(
			testhelper.Cert("metron.crt"),
			testhelper.Cert("metron.key"),
			testhelper.Cert("loggregator-ca.crt"),
			"doppler",
		)
		Expect(err).ToNot(HaveOccurred())

		serverCreds, err := plumbing.NewServerCredentials(
			testhelper.Cert("router.crt"),
			testhelper.Cert("router.key"),
			testhelper.Cert("loggregator-ca.crt"),
		)
		Expect(err).ToNot(HaveOccurred())

		config := buildAgentConfig("127.0.0.1", 1234)
		config.Zone = "something-bad"
		Expect(err).ToNot(HaveOccurred())

		mc := testhelper.NewMetricClient()

		app := app.NewV2App(
			&config,
			clientCreds,
			serverCreds,
			mc,
			app.WithV2Lookup(spyLookup.lookup),
		)
		go app.Start()

		Eventually(hasMetric(mc, "dropped", map[string]string{"direction": "egress", "metric_version": "2.0"})).Should(BeTrue())
		Eventually(hasMetric(mc, "dropped", map[string]string{"direction": "ingress", "metric_version": "2.0"})).Should(BeTrue())
		Eventually(hasMetric(mc, "egress", map[string]string{"metric_version": "2.0"})).Should(BeTrue())
		Eventually(hasMetric(mc, "ingress", map[string]string{"metric_version": "2.0"})).Should(BeTrue())
		Eventually(hasMetric(mc, "origin_mappings", map[string]string{"unit": "bytes/minute", "metric_version": "2.0"})).Should(BeTrue())
		Eventually(hasMetric(mc, "average_envelopes", map[string]string{"unit": "bytes/minute", "metric_version": "2.0", "loggregator": "v2"})).Should(BeTrue())
	})
})
