package app_test

import (
	"os"

	"code.cloudfoundry.org/loggregator-agent-release/src/cmd/otel-collector-manager/app"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Config", func() {
	BeforeEach(func() {
		os.Setenv("COLLECTOR_PID_FILE", "/a/path/to/somewhere")
		os.Setenv("COLLECTOR_BASE_CONFIG", "/a/path/to/somewhere/else")
		os.Setenv("COLLECTOR_RUNNING_CONFIG", "/a/path/to/somewhere/else/entirely")
		os.Setenv("COLLECTOR_BINARY", "/a/path/to/the/executable")
		os.Setenv("COLLECTOR_STDOUT_LOG", "/a/path/to/the/log")
		os.Setenv("COLLECTOR_STDERR_LOG", "/a/path/to/the/error_log")
	})

	It("Loads the config", func() {
		c, err := app.LoadConfig()

		Expect(err).ToNot(HaveOccurred())
		Expect(c.CollectorPidFile).To(Equal("/a/path/to/somewhere"))
		Expect(c.CollectorBaseConfig).To(Equal("/a/path/to/somewhere/else"))
		Expect(c.CollectorRunningConfig).To(Equal("/a/path/to/somewhere/else/entirely"))
		Expect(c.CollectorBinary).To(Equal("/a/path/to/the/executable"))
		Expect(c.CollectorStdoutLog).To(Equal("/a/path/to/the/log"))
		Expect(c.CollectorStderrLog).To(Equal("/a/path/to/the/error_log"))
	})

})
