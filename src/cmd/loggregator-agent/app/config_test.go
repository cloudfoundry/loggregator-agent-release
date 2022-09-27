package app_test

import (
	"os"

	"code.cloudfoundry.org/loggregator-agent-release/src/cmd/loggregator-agent/app"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Config", func() {
	BeforeEach(func() {
		os.Setenv("METRICS_CA_FILE_PATH", "something")
		os.Setenv("METRICS_CERT_FILE_PATH", "something")
		os.Setenv("METRICS_KEY_FILE_PATH", "something")
	})

	It("IDN encodes RouterAddrWithAZ", func() {
		os.Setenv("ROUTER_ADDR", "router-addr")
		os.Setenv("ROUTER_ADDR_WITH_AZ", "jedinečné.router-addr:1234")

		c, err := app.LoadConfig()

		Expect(err).ToNot(HaveOccurred())
		Expect(c.RouterAddrWithAZ).To(Equal("xn--jedinen-hya63a.router-addr:1234"))
	})

	It("strips @ from RouterAddrWithAZ to be DNS compatable", func() {
		os.Setenv("ROUTER_ADDR", "router-addr")
		os.Setenv("ROUTER_ADDR_WITH_AZ", "jedi@nečné.router-addr:1234")

		c, err := app.LoadConfig()

		Expect(err).ToNot(HaveOccurred())
		Expect(c.RouterAddrWithAZ).ToNot(ContainSubstring("@"))
	})

	It("source id defaults to metron", func() {
		os.Setenv("ROUTER_ADDR", "router-addr")
		c, err := app.LoadConfig()

		Expect(err).ToNot(HaveOccurred())
		Expect(c.MetricSourceID).To(Equal("metron"))
	})
})
