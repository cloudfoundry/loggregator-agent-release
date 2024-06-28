package syslog_test

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"

	"code.cloudfoundry.org/go-loggregator/v9/rfc5424"
	"code.cloudfoundry.org/go-loggregator/v9/rpc/loggregator_v2"
	metricsHelpers "code.cloudfoundry.org/go-metric-registry/testhelpers"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/syslog"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("HTTPSWriter", func() {
	var (
		drain *spyDrain

		binding *syslog.URLBinding
		tlsCfg  *tls.Config
		egress  *metricsHelpers.SpyMetric

		writer *syslog.HTTPSWriter
	)

	BeforeEach(func() {
		drain = newSpyDrain()
		binding = urlBinding(drain.URL, "test-app-id", "test-hostname")
		tlsCfg = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
		egress = &metricsHelpers.SpyMetric{}
	})

	AfterEach(func() {
		drain.Close()
	})

	JustBeforeEach(func() {
		var netCfg syslog.NetworkTimeoutConfig
		converter := syslog.NewConverter()

		var ok bool
		writer, ok = syslog.NewHTTPSWriter(binding, netCfg, tlsCfg, egress, converter).(*syslog.HTTPSWriter)
		Expect(ok).To(BeTrue())
	})

	Describe("Write", func() {
		It("converts the provided envelope to syslog messages and sends them to a drain via HTTPS", func() {
			Expect(writer.Write(buildLogEnvelope("APP", "1", "just a test", loggregator_v2.Log_OUT))).To(Succeed())
			Expect(drain.messages).To(HaveLen(1))
			Expect(writer.Write(buildGaugeEnvelope("1"))).To(Succeed())
			Expect(drain.messages).To(HaveLen(6))
		})

		It("sets Content-Length and Content-Type headers", func() {
			Expect(writer.Write(buildLogEnvelope("APP", "1", "just a test", loggregator_v2.Log_OUT))).To(Succeed())
			Expect(drain.headers[0]).To(HaveKeyWithValue("Content-Type", []string{"text/plain"}))
			Expect(drain.headers[0]).To(HaveKeyWithValue("Content-Length", []string{"118"}))
		})

		It("emits an egress metric for each message successfully sent", func() {
			Expect(egress.Value()).To(BeNumerically("==", 0))
			env := buildLogEnvelope("APP", "1", "just a test", loggregator_v2.Log_OUT)
			err := writer.Write(env)
			Expect(err).To(BeNil())
			Expect(egress.Value()).To(BeNumerically("==", 1))
		})

		Context("when TLS is misconfigured", func() {
			var err error

			BeforeEach(func() {
				tlsCfg = &tls.Config{} //nolint:gosec
			})

			JustBeforeEach(func() {
				env := buildLogEnvelope("APP", "1", "just a test", loggregator_v2.Log_OUT)
				err = writer.Write(env)
			})

			It("returns an error", func() {
				Expect(err).To(MatchError(ContainSubstring("tls: failed")))
			})

			It("does not increment the egress metric", func() {
				Expect(egress.Value()).To(BeNumerically("==", 0))
			})
		})

		Context("when the provided envelope is invalid", func() {
			var err error

			JustBeforeEach(func() {
				env := buildLogEnvelope("APP", "1", "just a test", loggregator_v2.Log_OUT)
				env.SourceId = " "
				err = writer.Write(env)
			})

			It("returns an error", func() {
				Expect(err).To(MatchError(ContainSubstring("Message cannot be serialized")))
			})

			It("does not increment the egress metric", func() {
				Expect(egress.Value()).To(BeNumerically("==", 0))
			})
		})

		Context("when the drain returns a status code that indicates a failure", func() {
			var err error

			JustBeforeEach(func() {
				drain.statusCode = http.StatusBadRequest
				env := buildLogEnvelope("APP", "1", "just a test", loggregator_v2.Log_OUT)
				err = writer.Write(env)
			})

			It("returns an error", func() {
				Expect(err).To(MatchError("syslog Writer: Post responded with 400 status code"))
			})

			It("does not increment the egress metric", func() {
				Expect(egress.Value()).To(BeNumerically("==", 0))
			})
		})

		Context("when there are credentials in the binding and the request fails", func() {
			BeforeEach(func() {
				binding = urlBinding(
					"http://user:password@localhost:0",
					"test-app-id",
					"test-hostname",
				)
			})

			It("returns an error that does not include the binding credentials", func() {
				env := buildLogEnvelope("APP", "1", "just a test", loggregator_v2.Log_OUT)
				err := writer.Write(env)
				Expect(err).To(HaveOccurred())

				Expect(err.Error()).NotTo(ContainSubstring("user"))
				Expect(err.Error()).NotTo(ContainSubstring("password"))
			})
		})
	})
})

type spyDrain struct {
	*httptest.Server
	messages   []*rfc5424.Message
	headers    []http.Header
	statusCode int
}

func newSpyDrain() *spyDrain {
	drain := &spyDrain{statusCode: http.StatusOK}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		message := &rfc5424.Message{}

		body, err := io.ReadAll(r.Body)
		Expect(err).ToNot(HaveOccurred())
		defer r.Body.Close()

		err = message.UnmarshalBinary(body)
		Expect(err).ToNot(HaveOccurred())

		drain.messages = append(drain.messages, message)
		drain.headers = append(drain.headers, r.Header)
		w.WriteHeader(drain.statusCode)
	})
	server := httptest.NewTLSServer(handler)
	drain.Server = server
	return drain
}

func urlBinding(u, appID, hostname string) *syslog.URLBinding {
	GinkgoHelper()

	parsedURL, err := url.Parse(u)
	Expect(err).NotTo(HaveOccurred())

	return &syslog.URLBinding{
		URL:      parsedURL,
		AppID:    appID,
		Hostname: hostname,
	}
}
