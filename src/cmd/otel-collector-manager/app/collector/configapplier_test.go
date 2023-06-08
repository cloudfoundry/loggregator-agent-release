package collector_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"

	"code.cloudfoundry.org/loggregator-agent-release/src/cmd/otel-collector-manager/app/collector"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("ConfigApplier", func() {
	var (
		ca *collector.ConfigApplier
	)

	Describe("Apply", func() {
		var (
			tempDirPath, pidFile string
			session              *gexec.Session
		)

		BeforeEach(func() {
			var err error
			tempDirPath, err = os.MkdirTemp("", "otel-collector-manager-test")
			Expect(err).ToNot(HaveOccurred())
		})

		Context("when the collector is running", func() {
			BeforeEach(func() {
				pidFile = filepath.Join(tempDirPath, "collector.pid")

				command := exec.Command("bash", "-c", `trap "echo Received SIGHUP" SIGHUP; echo pid=$$; while true; do sleep 0.5; done`)

				var err error
				session, err = gexec.Start(command, GinkgoWriter, GinkgoWriter)
				Expect(err).ShouldNot(HaveOccurred())

				re := regexp.MustCompile("pid=([0-9]+)")
				Eventually(session.Out).Should(gbytes.Say(re.String()))

				m := re.FindSubmatch(session.Out.Contents())
				Expect(len(m)).To(Equal(2))

				err = os.WriteFile(pidFile, m[1], 0600)
				Expect(err).ToNot(HaveOccurred())
			})

			It("sends a SIGHUP signal to the collector to reload config", func() {
				ca = collector.NewConfigApplier(pidFile)
				err := ca.Apply()
				Expect(err).ToNot(HaveOccurred())

				Eventually(session.Out).Should(gbytes.Say("Received SIGHUP"))
			})
		})

		Context("when the pid file cannot be read", func() {
			It("errors", func() {
				ca = collector.NewConfigApplier("/missing/pid/file")
				err := ca.Apply()
				Expect(err).To(HaveOccurred())
			})
		})

		Context("when the pid file is malformed", func() {
			BeforeEach(func() {
				tempDirPath, err := os.MkdirTemp("", "otel-collector-manager-test")
				Expect(err).ToNot(HaveOccurred())

				pidFile = filepath.Join(tempDirPath, "collector.pid")
				os.WriteFile(pidFile, []byte("malformed"), 0600)
			})

			It("errors", func() {
				ca = collector.NewConfigApplier(pidFile)
				err := ca.Apply()
				Expect(err).To(HaveOccurred())
			})
		})

		AfterEach(func() {
			gexec.TerminateAndWait()
			Expect(os.RemoveAll(tempDirPath)).To(Succeed())
		})
	})
})
