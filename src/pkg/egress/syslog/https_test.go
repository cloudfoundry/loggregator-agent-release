package syslog_test

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"

	"code.cloudfoundry.org/go-loggregator/v10/rfc5424"
	"code.cloudfoundry.org/go-loggregator/v10/rpc/loggregator_v2"
	metricsHelpers "code.cloudfoundry.org/go-metric-registry/testhelpers"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/syslog"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("HTTPWriter", func() {
	var (
		netConf          syslog.NetworkTimeoutConfig
		skipSSLTLSConfig = &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec
		}
		c = syslog.NewConverter()
	)

	It("errors when ssl validation is enabled", func() {
		drain := newMockOKDrain()

		b := buildURLBinding(drain.URL, "test-app-id", "test-hostname")

		writer := syslog.NewHTTPSWriter(
			b,
			netConf,
			&tls.Config{MinVersion: tls.VersionTLS12},
			&metricsHelpers.SpyMetric{},
			c,
		)

		env := buildLogEnvelope("APP", "1", "just a test", loggregator_v2.Log_OUT)
		Expect(writer.Write(env)).To(HaveOccurred()) //nolint
	})

	It("errors when the http POST fails", func() {
		drain := newMockErrorDrain()

		b := buildURLBinding(
			drain.URL,
			"test-app-id",
			"test-hostname",
		)

		writer := syslog.NewHTTPSWriter(
			b,
			netConf,
			skipSSLTLSConfig,
			&metricsHelpers.SpyMetric{},
			c,
		)
		env := buildLogEnvelope("APP", "1", "just a test", loggregator_v2.Log_OUT)
		Expect(writer.Write(env)).To(HaveOccurred())
	})

	It("does not leak creds when reporting a POST error", func() {
		b := buildURLBinding(
			"http://user:password@localhost:0",
			"test-app-id",
			"test-hostname",
		)

		writer := syslog.NewHTTPSWriter(
			b,
			netConf,
			skipSSLTLSConfig,
			&metricsHelpers.SpyMetric{},
			c,
		)

		env := buildLogEnvelope("APP", "1", "just a test", loggregator_v2.Log_OUT)
		err := writer.Write(env)
		Expect(err).To(HaveOccurred())

		Expect(err.Error()).ToNot(ContainSubstring("user"))
		Expect(err.Error()).ToNot(ContainSubstring("password"))
	})

	It("sets Content-Type to text/plain", func() {
		drain := newMockOKDrain()

		b := buildURLBinding(
			drain.URL,
			"test-app-id",
			"test-hostname",
		)

		writer := syslog.NewHTTPSWriter(
			b,
			netConf,
			skipSSLTLSConfig,
			&metricsHelpers.SpyMetric{},
			c,
		)

		env1 := buildLogEnvelope("APP", "1", "just a test", loggregator_v2.Log_OUT)
		Expect(writer.Write(env1)).To(Succeed())

		Expect(drain.messages).To(HaveLen(1))
		Expect(drain.headers).To(HaveLen(1))
		Expect(drain.headers[0]).To(HaveKeyWithValue("Content-Type", []string{"text/plain"}))
	})

	It("writes gauge metrics to the http drain", func() {
		drain := newMockOKDrain()

		b := buildURLBinding(
			drain.URL,
			"test-app-id",
			"test-hostname",
		)

		writer := syslog.NewHTTPSWriter(
			b,
			netConf,
			skipSSLTLSConfig,
			&metricsHelpers.SpyMetric{},
			c,
		)

		env1 := buildGaugeEnvelope("1")
		Expect(writer.Write(env1)).To(Succeed())

		Expect(drain.messages).To(HaveLen(5))

		Expect(drain.messages[0].StructuredData).To(HaveLen(1))
		Expect(drain.messages[0].StructuredData[0].ID).To(Equal("gauge@47450"))

		sdValues := func(msgs []*rfc5424.Message, name string) []string {
			var sd rfc5424.StructuredData
			for _, msg := range msgs {
				if msg.StructuredData[0].Parameters[0].Value == name {
					sd = msg.StructuredData[0]
					break
				}
			}

			data := make([]string, 0, 3)
			for _, param := range sd.Parameters {
				data = append(data, param.Value)
			}

			return data
		}

		Expect(sdValues(drain.messages, "cpu")).To(ConsistOf("cpu", "0.23", "percentage"))
		Expect(sdValues(drain.messages, "disk")).To(ConsistOf("disk", "1234", "bytes"))
		Expect(sdValues(drain.messages, "disk_quota")).To(ConsistOf("disk_quota", "1024", "bytes"))
		Expect(sdValues(drain.messages, "memory")).To(ConsistOf("memory", "5423", "bytes"))
		Expect(sdValues(drain.messages, "memory_quota")).To(ConsistOf("memory_quota", "8000", "bytes"))
	})

	It("writes counter metrics to the http drain", func() {
		drain := newMockOKDrain()

		b := buildURLBinding(
			drain.URL,
			"test-app-id",
			"test-hostname",
		)

		writer := syslog.NewHTTPSWriter(
			b,
			netConf,
			skipSSLTLSConfig,
			&metricsHelpers.SpyMetric{},
			c,
		)

		env1 := buildCounterEnvelope("1")
		Expect(writer.Write(env1)).To(Succeed())

		Expect(drain.messages).To(HaveLen(1))

		Expect(drain.messages[0].StructuredData).To(HaveLen(1))
		Expect(drain.messages[0].StructuredData[0].ID).To(Equal("counter@47450"))

		Expect(drain.messages[0].StructuredData[0].Parameters[0].Name).To(Equal("name"))
		Expect(drain.messages[0].StructuredData[0].Parameters[0].Value).To(Equal("some-counter"))

		Expect(drain.messages[0].StructuredData[0].Parameters[1].Name).To(Equal("total"))
		Expect(drain.messages[0].StructuredData[0].Parameters[1].Value).To(Equal("99"))

		Expect(drain.messages[0].StructuredData[0].Parameters[2].Name).To(Equal("delta"))
		Expect(drain.messages[0].StructuredData[0].Parameters[2].Value).To(Equal("1"))
	})

	It("emits an egress metric for each message", func() {
		drain := newMockOKDrain()

		b := buildURLBinding(
			drain.URL,
			"test-app-id",
			"test-hostname",
		)

		sm := &metricsHelpers.SpyMetric{}
		writer := syslog.NewHTTPSWriter(
			b,
			netConf,
			skipSSLTLSConfig,
			sm,
			c,
		)

		env := buildLogEnvelope("APP", "1", "just a test", loggregator_v2.Log_OUT)
		err := writer.Write(env)
		Expect(err).To(BeNil())

		Expect(sm.Value()).To(BeNumerically("==", 1))
	})

	It("ignores non-log envelopes", func() {
		drain := newMockOKDrain()

		b := buildURLBinding(
			drain.URL,
			"test-app-id",
			"test-hostname",
		)

		writer := syslog.NewHTTPSWriter(
			b,
			netConf,
			skipSSLTLSConfig,
			&metricsHelpers.SpyMetric{},
			c,
		)

		counterEnv := buildTimerEnvelope("1")
		logEnv := buildLogEnvelope("APP", "2", "just a test", loggregator_v2.Log_OUT)

		Expect(writer.Write(counterEnv)).To(Succeed())
		Expect(writer.Write(logEnv)).To(Succeed())
	})
})

type SpyDrain struct {
	mu sync.Mutex
	*httptest.Server
	messages     []*rfc5424.Message
	headers      []http.Header
	requestCount int
}

func (d *SpyDrain) Reset() {
	d.messages = nil
	d.headers = nil
	d.requestCount = 0
}

func (d *SpyDrain) appendMessage(message *rfc5424.Message) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.messages = append(d.messages, message)
}

func (d *SpyDrain) appendHeader(header http.Header) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.headers = append(d.headers, header)
}

func (d *SpyDrain) getMessagesSize() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.messages)
}

func (d *SpyDrain) getRequestCount() int {
	return d.requestCount
}
func newMockOKDrain() *SpyDrain {
	return newMockDrain(http.StatusOK)
}
func newMockErrorDrain() *SpyDrain {
	return newMockDrain(http.StatusBadRequest)
}
func newMockDrain(status int) *SpyDrain {
	drain := &SpyDrain{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		message := &rfc5424.Message{}

		body, err := io.ReadAll(r.Body)
		Expect(err).ToNot(HaveOccurred())
		defer r.Body.Close()

		err = message.UnmarshalBinary(body)
		Expect(err).ToNot(HaveOccurred())

		drain.appendMessage(message)
		drain.appendHeader(r.Header)
		w.WriteHeader(status)
	})
	server := httptest.NewTLSServer(handler)
	drain.Server = server
	return drain
}

func buildURLBinding(u, appID, hostname string) *syslog.URLBinding {
	parsedURL, _ := url.Parse(u)

	return &syslog.URLBinding{
		URL:      parsedURL,
		AppID:    appID,
		Hostname: hostname,
	}
}
