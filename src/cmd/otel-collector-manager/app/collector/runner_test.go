package collector_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"

	"code.cloudfoundry.org/loggregator-agent-release/src/cmd/otel-collector-manager/app/collector"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/sirupsen/logrus"
)

var _ = Describe("Runner", func() {
	var (
		r      *collector.Runner
		l      *logrus.Logger
		stdout *gbytes.Buffer
		stderr *gbytes.Buffer

		tempDirPath, pidFile string
		sd                   time.Duration
	)

	BeforeEach(func() {
		l = logrus.New()
		l.SetOutput(GinkgoWriter)

		var err error
		tempDirPath, err = os.MkdirTemp("", "otel-collector-manager-test")
		Expect(err).ToNot(HaveOccurred())
		pidFile = filepath.Join(tempDirPath, "collector.pid")
		sd, err = time.ParseDuration("50ms")
		Expect(err).ToNot(HaveOccurred())

		stdout = gbytes.NewBuffer()
		stderr = gbytes.NewBuffer()
	})

	AfterEach(func() {
		Expect(os.RemoveAll(tempDirPath)).To(Succeed())
	})

	Describe("RemovePidFile", func() {
		Context("when the pid file exists", func() {
			BeforeEach(func() {
				err := os.WriteFile(pidFile, []byte("1234"), 0600)
				Expect(err).ToNot(HaveOccurred())
			})

			It("removes the pid file", func() {
				r = collector.NewRunner(pidFile, "", nil, stdout, stderr, sd, l)
				r.RemovePidFile()

				_, err := os.Stat(pidFile)
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("IsRunning", func() {
		Context("when the collector is running", func() {
			var (
				session *gexec.Session
			)

			BeforeEach(func() {
				command := exec.Command("bash", "-c", "echo pid=$$; sleep infinity")

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

			It("returns true", func() {
				r = collector.NewRunner(pidFile, "", nil, stdout, stderr, sd, l)
				Expect(r.IsRunning()).To(BeTrue())
			})

			Context("and is then stopped", func() {
				BeforeEach(func() {
					gexec.TerminateAndWait()
				})
				It("returns false", func() {
					r = collector.NewRunner(pidFile, "", nil, stdout, stderr, sd, l)
					Expect(r.IsRunning()).To(BeFalse())
				})
			})

			AfterEach(func() {
				gexec.TerminateAndWait()
			})
		})
		Context("when the pid file cannot be read", func() {
			It("returns false", func() {
				r = collector.NewRunner("/missing/pid/file", "", nil, stdout, stderr, sd, l)
				Expect(r.IsRunning()).To(BeFalse())
			})
		})
		Context("when the pid file is malformed", func() {
			BeforeEach(func() {
				os.WriteFile(pidFile, []byte("malformed"), 0600)
			})

			It("returns false", func() {
				r = collector.NewRunner(pidFile, "", nil, stdout, stderr, sd, l)
				Expect(r.IsRunning()).To(BeFalse())
			})
		})
	})

	Describe("Start", func() {
		AfterEach(func() {
			_ = r.Stop()
		})

		Context("when it is run for the first time", func() {
			It("starts the process", func() {
				Expect(pidFile).ToNot(BeAnExistingFile())

				r = collector.NewRunner(pidFile, "bash", []string{"-c", "sleep infinity"}, stdout, stderr, sd, l)
				Expect(r.Start()).To(Succeed())
				Eventually(r.IsRunning).Should(BeTrue())
				Eventually(pidFile).Should(BeAnExistingFile())
			})

			It("writes a pid file", func() {
				r = collector.NewRunner(pidFile, "bash", []string{"-c", "sleep infinity"}, stdout, stderr, sd, l)
				Expect(r.Start()).To(Succeed())

				pid, err := os.ReadFile(pidFile)
				Expect(err).ToNot(HaveOccurred())

				Expect(pid).To(MatchRegexp("[0-9]+"))
			})

			It("redirects stdout and stderr", func() {
				r = collector.NewRunner(pidFile, "bash", []string{"-c", "echo starting && echo error >&2 && sleep infinity"}, stdout, stderr, sd, l)
				Expect(r.Start()).To(Succeed())

				Eventually(stdout).Should(gbytes.Say("starting"))
				Eventually(stderr).Should(gbytes.Say("error"))
			})

			Context("when the process fails to start", func() {
				It("errors", func() {
					r = collector.NewRunner(pidFile, "non-existent-command", []string{}, stdout, stderr, sd, l)
					Expect(r.Start()).To(HaveOccurred())
				})
			})

			Context("when the pid file cannot be written", func() {
				BeforeEach(func() {
					Expect(os.Mkdir(pidFile, 0700)).To(Succeed())
				})
				It("errors", func() {
					r = collector.NewRunner(pidFile, "bash", []string{"-c", "sleep infinity"}, stdout, stderr, sd, l)
					Expect(r.Start()).To(HaveOccurred())
				})
			})

			Context("when the process errors", func() {
				var o *gbytes.Buffer
				BeforeEach(func() {
					o = gbytes.NewBuffer()
					l.SetOutput(o)
				})

				It("logs the error", func() {
					r = collector.NewRunner(pidFile, "bash", []string{"-c", "exit 1"}, stdout, stderr, sd, l)
					Expect(r.Start()).To(Succeed())
					Eventually(o).Should(gbytes.Say("process exited with an error.*exit status 1"))
				})
			})
		})
	})

	Describe("Stop", func() {
		Context("when the process is running", func() {
			BeforeEach(func() {
				r = collector.NewRunner(pidFile, "bash", []string{"-c", "sleep infinity"}, stdout, stderr, sd, l)
				r.Start()
				Eventually(r.IsRunning).Should(BeTrue())
			})

			It("stops the process", func() {
				Expect(r.Stop()).To(Succeed())
				Eventually(r.IsRunning).Should(BeFalse())
			})
		})

		Context("when the process ignores SIGTERM", func() {
			BeforeEach(func() {
				r = collector.NewRunner(pidFile, "bash", []string{"-c", "trap '' SIGTERM; while true; do sleep 0.5; done"}, stdout, stderr, sd, l)
				r.Start()
				Consistently(r.IsRunning).Should(BeTrue())
			})

			It("stops the process", func() {
				Expect(r.Stop()).To(Succeed())
				Eventually(r.IsRunning).Should(BeFalse())
			})
		})

		Context("when the pid file cannot be read", func() {
			It("errors", func() {
				r = collector.NewRunner("/missing/pid/file", "", nil, stdout, stderr, sd, l)
				Expect(r.Stop()).To(HaveOccurred())
			})
		})

		Context("when the pid file is malformed", func() {
			BeforeEach(func() {
				os.WriteFile(pidFile, []byte("malformed"), 0600)
			})

			It("errors", func() {
				r = collector.NewRunner(pidFile, "", nil, stdout, stderr, sd, l)
				Expect(r.Stop()).To(HaveOccurred())
			})
		})

		Context("when the process has already stopped running", func() {
			BeforeEach(func() {
				r = collector.NewRunner(pidFile, "bash", []string{"-c", "sleep infinity"}, stdout, stderr, sd, l)
				r.Start()
				Eventually(r.IsRunning).Should(BeTrue())
				Expect(r.Stop()).To(Succeed())
				Eventually(r.IsRunning).Should(BeFalse())
			})

			It("does not error", func() {
				Expect(r.Stop()).To(Succeed())
			})
		})

	})
})
