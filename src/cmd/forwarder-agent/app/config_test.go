package app_test

import (
	"os"

	"code.cloudfoundry.org/loggregator-agent-release/src/cmd/forwarder-agent/app"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Config", func() {
	BeforeEach(func() {
		os.Setenv("AGENT_CA_FILE_PATH", "ca.crt")
		os.Setenv("AGENT_CERT_FILE_PATH", "agent.crt")
		os.Setenv("AGENT_KEY_FILE_PATH", "agent.key")
		os.Setenv("METRICS_CA_FILE_PATH", "metrics-ca.crt")
		os.Setenv("METRICS_CERT_FILE_PATH", "metrics.crt")
		os.Setenv("METRICS_KEY_FILE_PATH", "metrics.key")
	})

	It("defaults the OTel Collector host to localhost", func() {
		os.Setenv("OTEL_COLLECTOR_HOST", "")

		Expect(app.LoadConfig().OTelCollectorHost).To(Equal("127.0.0.1"))
	})

	It("configures the OTel Collector host", func() {
		os.Setenv("OTEL_COLLECTOR_HOST", "otel-collector")

		Expect(app.LoadConfig().OTelCollectorHost).To(Equal("otel-collector"))
	})
})
