package agent_test

import (
	"fmt"
	"io"
	"net/http"

	"code.cloudfoundry.org/loggregator-agent-release/src/internal/testhelper"
	"code.cloudfoundry.org/tlsconfig"

	"code.cloudfoundry.org/loggregator-agent-release/src/internal/testservers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Agent Health Endpoint", func() {
	It("returns health metrics", func() {
		testCerts := testhelper.GenerateCerts("loggregatorCA")
		consumerServer, err := NewServer(testCerts)
		Expect(err).ToNot(HaveOccurred())
		defer consumerServer.Stop() // nolint:errcheck
		agentCleanup, agentPorts := testservers.StartAgent(
			testservers.BuildAgentConfig("127.0.0.1", consumerServer.Port(), testCerts),
		)
		defer agentCleanup()

		tlsConfig, err := tlsconfig.Build(
			tlsconfig.WithIdentityFromFile(testCerts.Cert("client"), testCerts.Key("client")),
		).Client(tlsconfig.WithAuthorityFromFile(testCerts.CA()))
		Expect(err).ToNot(HaveOccurred())

		tlsClient := &http.Client{Transport: &http.Transport{TLSClientConfig: tlsConfig}}

		resp, err := tlsClient.Get(fmt.Sprintf("https://127.0.0.1:%d/metrics", agentPorts.Metrics))
		Expect(err).ToNot(HaveOccurred())
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		body, err := io.ReadAll(resp.Body)
		Expect(err).ToNot(HaveOccurred())

		Expect(body).To(ContainSubstring("doppler_v1_streams"))
		Expect(body).To(ContainSubstring("doppler_v2_streams"))
		Expect(body).To(ContainSubstring("doppler_connections"))
	})
})
