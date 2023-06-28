package main_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"

	"code.cloudfoundry.org/loggregator-agent-release/src/internal/testhelper"
	"code.cloudfoundry.org/tlsconfig"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"gopkg.in/yaml.v2"
)

var _ = Describe("Manager", func() {
	var (
		tempDirPath, collectorBinary, baseConfig,
		runningConfig, pidFile, stdoutLog, stderrLog string

		session *gexec.Session
		command *exec.Cmd

		bindingCache *fakeBindingCache
	)

	BeforeEach(func() {
		var err error
		tempDirPath, err = os.MkdirTemp("", "otel-collector-manager-test")
		Expect(err).ToNot(HaveOccurred())

		cacheCerts := testhelper.GenerateCerts("binding-cache-ca")
		bindingCache = &fakeBindingCache{}
		bindingCache.startTLS(cacheCerts)

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
			fmt.Sprintf("CACHE_URL=%s", bindingCache.URL),
			fmt.Sprintf("CACHE_CA_FILE_PATH=%s", cacheCerts.CA()),
			fmt.Sprintf("CACHE_CERT_FILE_PATH=%s", cacheCerts.Cert("collector-manager")),
			fmt.Sprintf("CACHE_KEY_FILE_PATH=%s", cacheCerts.Key("collector-manager")),
			"CACHE_COMMON_NAME=binding-cache",
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

	It("writes a config file with the exporters received from the cache", func() {
		var err error
		session, err = gexec.Start(command, GinkgoWriter, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() map[string]any {
			b, _ := os.ReadFile(runningConfig)
			var result map[string]map[string]any
			_ = yaml.Unmarshal(b, &result)
			return result["exporters"]
		}).Should(Equal(map[string]any{
			"logging": map[any]any{
				"verbosity": "detailed",
			},
		}))
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

type fakeBindingCache struct {
	*httptest.Server
}

func (f *fakeBindingCache) startTLS(testCerts *testhelper.TestCerts) {
	tlsConfig, err := tlsconfig.Build(
		tlsconfig.WithInternalServiceDefaults(),
		tlsconfig.WithIdentityFromFile(
			testCerts.Cert("binding-cache"),
			testCerts.Key("binding-cache"),
		),
	).Server(
		tlsconfig.WithClientAuthenticationFromFile(testCerts.CA()),
	)

	Expect(err).ToNot(HaveOccurred())

	f.Server = httptest.NewUnstartedServer(f)
	f.Server.TLS = tlsConfig
	f.Server.StartTLS()
}

func (f *fakeBindingCache) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var (
		results []byte
		err     error
	)

	switch r.URL.Path {
	case "/v2/aggregatemetric":
		cfg := map[string]any{
			"logging": map[string]any{
				"verbosity": "detailed",
			},
		}
		results, err = json.Marshal(cfg)
	default:
		w.WriteHeader(500)
		return
	}

	if err != nil {
		w.WriteHeader(500)
		return
	}

	_, _ = w.Write(results)
}
