package app_test

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"time"

	"code.cloudfoundry.org/tlsconfig"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metricsHelpers "code.cloudfoundry.org/go-metric-registry/testhelpers"
	"code.cloudfoundry.org/loggregator-agent-release/src/cmd/syslog-binding-cache/app"
	"code.cloudfoundry.org/loggregator-agent-release/src/internal/testhelper"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/binding"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/config"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/plumbing"
)

var _ = Describe("App", func() {
	const sbcCN = "binding-cache"

	var (
		capi *fakeCC

		sbcPort     int
		pprofPort   int
		metricsPort int

		sbcCerts *testhelper.TestCerts

		sbcCfg     app.Config
		sbcMetrics *metricsHelpers.SpyMetricsRegistry
		sbcLogr    *log.Logger
		sbc        *app.SyslogBindingCache

		client *http.Client
	)

	BeforeEach(func() {
		r := results{
			{
				Url: "syslog://drain-a",
				Credentials: []binding.Credentials{
					{
						Cert: "cert", Key: "key", Apps: []binding.App{{Hostname: "org.space.app-name-1", AppID: "app-id-1"}},
					},
				},
			},
			{
				Url: "syslog://drain-b",
				Credentials: []binding.Credentials{
					{
						Cert: "cert", Key: "key", Apps: []binding.App{{Hostname: "org.space.app-name-1", AppID: "app-id-1"}},
					},
				},
			},
			{
				Url: "syslog://drain-c",
				Credentials: []binding.Credentials{
					{
						Cert: "cert", Key: "key", Apps: []binding.App{{Hostname: "org.space.app-name-2", AppID: "app-id-2"}},
					},
				},
			},
			{
				Url: "syslog://drain-d",
				Credentials: []binding.Credentials{
					{
						Cert: "cert", Key: "key", Apps: []binding.App{{Hostname: "org.space.app-name-2", AppID: "app-id-2"}},
					},
				},
			},
		}

		capi = &fakeCC{
			results: r,
		}
		capiCerts := testhelper.GenerateCerts("capi-ca")
		capi.startTLS(capiCerts)

		sbcPort = 30000 + GinkgoParallelProcess()
		pprofPort = 31000 + GinkgoParallelProcess()
		metricsPort = 32000 + GinkgoParallelProcess()

		sbcCerts = testhelper.GenerateCerts("binding-cache-ca")
		sbcCfg = app.Config{
			APIURL:             capi.URL,
			APIPollingInterval: 10 * time.Millisecond,
			APIBatchSize:       1000,
			APICAFile:          capiCerts.CA(),
			APICertFile:        capiCerts.Cert("capi"),
			APIKeyFile:         capiCerts.Key("capi"),
			APICommonName:      "capi",
			CacheCAFile:        sbcCerts.CA(),
			CacheCertFile:      sbcCerts.Cert(sbcCN),
			CacheKeyFile:       sbcCerts.Key(sbcCN),
			CacheCommonName:    sbcCN,
			CachePort:          sbcPort,
			AggregateDrains:    []string{"syslog://drain-e", "syslog://drain-f"},
			MetricsServer: config.MetricsServer{
				Port:      uint16(metricsPort),
				CAFile:    sbcCerts.CA(),
				CertFile:  sbcCerts.Cert("metron"),
				KeyFile:   sbcCerts.Key("metron"),
				PprofPort: uint16(pprofPort),
			},
		}
		sbcMetrics = metricsHelpers.NewMetricsRegistry()
		sbcLogr = log.New(GinkgoWriter, "", log.LstdFlags)

		client = plumbing.NewTLSHTTPClient(
			sbcCerts.Cert(sbcCN),
			sbcCerts.Key(sbcCN),
			sbcCerts.CA(),
			sbcCN,
		)
	})

	JustBeforeEach(func() {
		sbc = app.NewSyslogBindingCache(sbcCfg, sbcMetrics, sbcLogr)
		go sbc.Run()

		// Make sure the server has started to avoid an error when trying to
		// stop it in AfterEach.
		Eventually(func() bool {
			resp, err := client.Get(fmt.Sprintf("https://localhost:%d/aggregate", sbcPort))
			if err != nil {
				return false
			}
			defer resp.Body.Close()
			return resp.StatusCode == 200
		}, 10).Should(BeTrue())
	})

	AfterEach(func() {
		sbc.Stop()
		capi.CloseClientConnections()
		capi.Close()
	})

	It("polls CAPI on an interval for results", func() {
		Eventually(capi.numRequests).Should(BeNumerically(">=", 2))
	})

	It("has an HTTP endpoint that returns bindings", func() {
		addr := fmt.Sprintf("https://localhost:%d/v2/bindings", sbcPort)

		var resp *http.Response
		Eventually(func() error {
			var err error
			resp, err = client.Get(addr)
			return err
		}).Should(Succeed())
		defer resp.Body.Close()

		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		body, err := io.ReadAll(resp.Body)
		Expect(err).ToNot(HaveOccurred())

		var results []binding.Binding
		err = json.Unmarshal(body, &results)
		Expect(err).ToNot(HaveOccurred())

		Expect(results).To(HaveLen(4))
		b := findBindings(results, "app-id-1")
		Expect(b[0].Url).To(Equal("syslog://drain-a"))
		Expect(b[1].Url).To(Equal("syslog://drain-b"))
		Expect(b[0].Credentials[0].Apps[0].Hostname).To(Equal("org.space.app-name-1"))

		b = findBindings(results, "app-id-2")
		Expect(b[0].Url).To(Equal("syslog://drain-c"))
		Expect(b[1].Url).To(Equal("syslog://drain-d"))
		Expect(b[0].Credentials[0].Apps[0].Hostname).To(Equal("org.space.app-name-2"))
	})

	It("has an HTTP endpoint that returns aggregate drains", func() {
		addr := fmt.Sprintf("https://localhost:%d/aggregate", sbcPort)

		var resp *http.Response
		Eventually(func() error {
			var err error
			resp, err = client.Get(addr)
			return err
		}).Should(Succeed())
		defer resp.Body.Close()

		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		body, err := io.ReadAll(resp.Body)
		Expect(err).ToNot(HaveOccurred())

		var result []binding.LegacyBinding
		err = json.Unmarshal(body, &result)
		Expect(err).ToNot(HaveOccurred())

		Expect(result).To(HaveLen(1))
		Expect(result[0].Drains).To(ConsistOf("syslog://drain-e", "syslog://drain-f"))
	})

	It("has an HTTP endpoint that returns legacy bindings", func() {
		addr := fmt.Sprintf("https://localhost:%d/bindings", sbcPort)

		var resp *http.Response
		Eventually(func() error {
			var err error
			resp, err = client.Get(addr)
			return err
		}).Should(Succeed())
		defer resp.Body.Close()

		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		body, err := io.ReadAll(resp.Body)
		Expect(err).ToNot(HaveOccurred())

		var results []binding.LegacyBinding
		err = json.Unmarshal(body, &results)
		Expect(err).ToNot(HaveOccurred())

		Expect(results).To(HaveLen(2))
		result1 := binding.LegacyBinding{AppID: "app-id-1", Drains: []string{"syslog://drain-a", "syslog://drain-b"}, Hostname: "org.space.app-name-1", V2Available: true}
		result2 := binding.LegacyBinding{AppID: "app-id-2", Drains: []string{"syslog://drain-c", "syslog://drain-d"}, Hostname: "org.space.app-name-2", V2Available: true}
		Expect(results).Should(ConsistOf(result1, result2))
	})

	Context("when debug configuration is enabled", func() {
		BeforeEach(func() {
			sbcCfg.MetricsServer.DebugMetrics = true
		})

		It("registers debug metrics", func() {
			Eventually(sbcMetrics.GetDebugMetricsEnabled).Should(BeTrue())
		})

		It("serves a pprof endpoint", func() {
			var resp *http.Response
			Eventually(func() error {
				var err error
				resp, err = http.Get(fmt.Sprintf("http://127.0.0.1:%d/debug/pprof/", pprofPort))
				return err
			}).Should(BeNil())
			Expect(resp.StatusCode).To(Equal(200))
		})
	})
})

type results []binding.Binding

type fakeCC struct {
	*httptest.Server
	count           int
	called          int64
	withEmptyResult bool
	results         results
}

func (f *fakeCC) startTLS(testCerts *testhelper.TestCerts) {
	tlsConfig, err := tlsconfig.Build(
		tlsconfig.WithInternalServiceDefaults(),
		tlsconfig.WithIdentityFromFile(
			testCerts.Cert("capi"),
			testCerts.Key("capi"),
		),
	).Server(
		tlsconfig.WithClientAuthenticationFromFile(testCerts.CA()),
	)

	Expect(err).ToNot(HaveOccurred())

	f.Server = httptest.NewUnstartedServer(f)
	f.Server.TLS = tlsConfig
	f.Server.StartTLS()
}

func (f *fakeCC) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	atomic.AddInt64(&f.called, 1)
	if r.URL.Path != "/internal/v5/syslog_drain_urls" {
		w.WriteHeader(500)
		return
	}

	if r.URL.Query().Get("batch_size") != "1000" {
		w.WriteHeader(500)
		return
	}

	f.serveWithResults(w, r)
}

func (f *fakeCC) serveWithResults(w http.ResponseWriter, r *http.Request) {
	resultData, err := json.Marshal(struct {
		Results results `json:"results"`
	}{
		Results: f.results,
	})
	if err != nil {
		w.WriteHeader(500)
		return
	}

	if f.count > 0 {
		resultData = []byte(`{"results": {}}`)
	}

	_, err = w.Write(resultData)
	if err != nil {
		w.WriteHeader(500)
		return
	}

	if f.withEmptyResult {
		f.count++
	}
}

func (f *fakeCC) numRequests() int64 {
	return atomic.LoadInt64(&f.called)
}

func findBindings(bindings []binding.Binding, appID string) []binding.Binding {
	var bindingResult []binding.Binding
	for _, b := range bindings {
		for _, c := range b.Credentials {
			for _, a := range c.Apps {
				if appID == a.AppID {
					bindingResult = append(bindingResult, b)
				}
			}
		}

	}
	return bindingResult
}
