package main_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("Manager", func() {
	var (
		tempDirPath, collectorBinary, baseConfig,
		runningConfig, pidFile, stdoutLog, stderrLog string

		session *gexec.Session
		command *exec.Cmd
	)

	BeforeEach(func() {
		var err error
		tempDirPath, err = os.MkdirTemp("", "otel-collector-manager-test")
		Expect(err).ToNot(HaveOccurred())

		collectorBinary = filepath.Join(tempDirPath, "collector")

		//nolint:gosec
		Expect(os.WriteFile(collectorBinary, []byte(`#!/bin/bash
echo starting stdout
echo starting stderr >&2
`), 0700)).To(Succeed())

		baseConfig = filepath.Join(tempDirPath, "base.yml")
		Expect(os.WriteFile(baseConfig, []byte{}, 0600)).To(Succeed())

		runningConfig = filepath.Join(tempDirPath, "config.yml")
		pidFile = filepath.Join(tempDirPath, "collector.pid")

		stdoutLog = filepath.Join(tempDirPath, "stdout.log")
		stderrLog = filepath.Join(tempDirPath, "stderr.log")

		command = exec.Command(otelCollectorManagerPath)
		command.Env = []string{
			fmt.Sprintf("COLLECTOR_PID_FILE=%s", pidFile),
			fmt.Sprintf("COLLECTOR_BASE_CONFIG=%s", baseConfig),
			fmt.Sprintf("COLLECTOR_RUNNING_CONFIG=%s", runningConfig),
			fmt.Sprintf("COLLECTOR_BINARY=%s", collectorBinary),
			fmt.Sprintf("COLLECTOR_STDOUT_LOG=%s", stdoutLog),
			fmt.Sprintf("COLLECTOR_STDERR_LOG=%s", stderrLog),
		}
	})

	AfterEach(func() {
		Expect(os.RemoveAll(tempDirPath)).To(Succeed())
		if session != nil {
			session.Terminate().Wait()
		}
	})

	var fileContents = func(f string) func() string {
		return func() string {
			b, _ := os.ReadFile(f)
			return string(b)
		}
	}

	It("writes to the log files", func() {
		var err error
		session, err = gexec.Start(command, GinkgoWriter, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())

		Eventually(fileContents(stdoutLog)).Should(ContainSubstring("starting stdout"))
		Eventually(fileContents(stderrLog)).Should(ContainSubstring("starting stderr"))
	})

	Context("when the log files already contain logs", func() {
		BeforeEach(func() {
			Expect(os.WriteFile(stdoutLog, []byte("existing stdout log lines\n"), 0600)).To(Succeed())
			Expect(os.WriteFile(stderrLog, []byte("existing stderr log lines\n"), 0600)).To(Succeed())
		})

		It("does not truncate them", func() {
			var err error
			session, err = gexec.Start(command, GinkgoWriter, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())

			Eventually(fileContents(stdoutLog)).Should(ContainSubstring("starting stdout"))
			Consistently(fileContents(stdoutLog)).Should(ContainSubstring("existing stdout log lines"))

			Eventually(fileContents(stderrLog)).Should(ContainSubstring("starting stderr"))
			Consistently(fileContents(stderrLog)).Should(ContainSubstring("existing stderr log lines"))
		})
	})

	It("shuts down cleanly when sent a signal", func() {
		var err error
		session, err = gexec.Start(command, GinkgoWriter, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())

		Eventually(session.Out).Should(gbytes.Say("Starting OTel Manager"))
		session.Terminate().Wait()

		Eventually(session.Out).Should(gbytes.Say("Stopping OTel Manager"))
		Eventually(session.Out).Should(gbytes.Say("OTel Manager Stopped"))
	})

})
