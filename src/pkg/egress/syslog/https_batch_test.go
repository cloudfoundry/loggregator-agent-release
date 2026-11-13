package syslog_test

import (
	"bytes"
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"time"

	"code.cloudfoundry.org/go-loggregator/v10/rfc5424"
	"code.cloudfoundry.org/go-loggregator/v10/rpc/loggregator_v2"
	metricsHelpers "code.cloudfoundry.org/go-metric-registry/testhelpers"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/binding/bindingfakes"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/syslog"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var stringTo256Chars string

func init() {
	//With the rest of the syslog, this results in a syslogenvelope of the size 400
	for i := 0; i < 256; i++ {
		stringTo256Chars += "a"
	}
}

var _ = Describe("HTTPS_batch", func() {
	var (
		netConf          syslog.NetworkTimeoutConfig
		skipSSLTLSConfig = &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec
		}
		c            = syslog.NewConverter()
		drain        *SpyDrain
		b            *syslog.URLBinding
		writer       egress.WriteCloser
		sendInterval time.Duration
		waitTime     time.Duration
	)

	BeforeEach(func() {
		drain = newBatchMockDrain(200)
		drain.Reset()
		sendInterval = 100 * time.Millisecond
		waitTime = sendInterval * 10
		b = buildURLBinding(
			drain.URL,
			"test-app-id",
			"test-hostname",
		)
		writer = syslog.NewHTTPSBatchWriter(
			b,
			netConf,
			skipSSLTLSConfig,
			&metricsHelpers.SpyMetric{},
			c,
			syslog.WithBatchSize(1000),
			syslog.WithSendInterval(sendInterval),
		)
		rw := writer.(syslog.InternalRetryWriter)
		rw.ConfigureRetry(
			func(i int) time.Duration {
				return 0
			},
			0) // No retries
	})

	AfterEach(func() {
		writer.Close()
	})

	It("testing simple appending of one log", func() {
		env1 := buildLogEnvelope("APP", "1", "message 1", loggregator_v2.Log_OUT)
		Expect(writer.Write(env1)).To(Succeed())
		env2 := buildLogEnvelope("APP", "2", "message 2", loggregator_v2.Log_OUT)
		Expect(writer.Write(env2)).To(Succeed())
		Eventually(drain.getMessagesSize, sendInterval+waitTime).Should(Equal(2))

		expected := &rfc5424.Message{
			AppName:   "test-app-id",
			Hostname:  "test-hostname",
			Priority:  rfc5424.Priority(14),
			ProcessID: "[APP/1]",
			Message:   []byte("message 1\n"),
		}
		Expect(drain.messages[0].AppName).To(Equal(expected.AppName))
		Expect(drain.messages[0].Hostname).To(Equal(expected.Hostname))
		Expect(drain.messages[0].Priority).To(BeEquivalentTo(expected.Priority))
		Expect(drain.messages[0].ProcessID).To(Equal(expected.ProcessID))
		Expect(drain.messages[0].Message).To(Equal(expected.Message))
		expected = &rfc5424.Message{
			AppName:   "test-app-id",
			Hostname:  "test-hostname",
			Priority:  rfc5424.Priority(14),
			ProcessID: "[APP/2]",
			Message:   []byte("message 2\n"),
		}
		Expect(drain.messages[1].AppName).To(Equal(expected.AppName))
		Expect(drain.messages[1].Hostname).To(Equal(expected.Hostname))
		Expect(drain.messages[1].Priority).To(BeEquivalentTo(expected.Priority))
		Expect(drain.messages[1].ProcessID).To(Equal(expected.ProcessID))
		Expect(drain.messages[1].Message).To(Equal(expected.Message))
	})

	It("test batch dispatching with all logs in a given timeframe", func() {
		env1 := buildLogEnvelope("APP", "1", "short message", loggregator_v2.Log_OUT)
		for i := 0; i < 5; i++ {
			Expect(writer.Write(env1)).To(Succeed())
		}
		Expect(drain.getMessagesSize()).To(Equal(0))
		Eventually(drain.getMessagesSize, sendInterval+waitTime).Should(Equal(5))
	})

	It("triggers multiple flushes when exceeding the batch", func() {
		env := buildLogEnvelope("APP", "1", "string to get log to 400 characters:"+stringTo256Chars, loggregator_v2.Log_OUT)
		for i := 0; i < 15; i++ {
			Expect(writer.Write(env)).To(Succeed())
		}
		// This is a less flaky approach to test for batch size triggered sends:
		// with only Time based sends, it either takes
		Eventually(drain.getMessagesSize, sendInterval+waitTime).Should(Equal(15))
		Expect(drain.getRequestCount()).Should(BeNumerically(">=", 3))
		// Batches should contain more than one message -> fewer requests than messages
		Expect(drain.getRequestCount()).Should(BeNumerically("<=", 10))
	})

	It("test for hanging after some ticks", func() {
		// This test will not succeed on the timer based implementation,
		// it works fine with a ticker based implementation.
		env1 := buildLogEnvelope("APP", "1", "only a short test message", loggregator_v2.Log_OUT)
		for i := 0; i < 5; i++ {
			Expect(writer.Write(env1)).To(Succeed())
			time.Sleep((sendInterval * 2) + (sendInterval / 5)) // this sleeps at least 2 ticks, to trigger once without events
		}
		Eventually(drain.getMessagesSize, sendInterval+waitTime).Should(Equal(5))
	})

	Describe("IP blacklist enforcement", func() {
		var (
			fakeChecker *bindingfakes.FakeIPChecker
			env         *loggregator_v2.Envelope
		)

		BeforeEach(func() {
			fakeChecker = &bindingfakes.FakeIPChecker{}
			env = buildLogEnvelope("APP", "1", "just a test", loggregator_v2.Log_OUT)
		})

		It("fails write with ErrBlockedAddress when IPChecker blocks loopback", func() {
			drain := newBatchMockDrain(http.StatusOK)
			defer drain.Close()

			b := buildURLBinding(drain.URL, "test-app-id", "test-hostname")
			fakeChecker.CheckBlacklistReturns(errors.New("blacklisted"))

			writer := syslog.NewHTTPSBatchWriter(
				b,
				syslog.NetworkTimeoutConfig{
					DialTimeout: 100 * time.Millisecond,
					IPChecker:   fakeChecker,
				},
				skipSSLTLSConfig,
				&metricsHelpers.SpyMetric{},
				c,
				syslog.WithSendInterval(10*time.Millisecond),
			)
			defer writer.Close()

			rw := writer.(syslog.InternalRetryWriter)
			rw.ConfigureRetry(func(i int) time.Duration { return 0 }, 0)

			batchWriter := writer.(*syslog.HTTPSBatchWriter)
			err := batchWriter.HTTPSWriter.Write(env)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, syslog.ErrBlockedAddress)).To(BeTrue())

			Expect(writer.Write(env)).To(Succeed())
			Consistently(drain.getMessagesSize, 50*time.Millisecond).Should(Equal(0))
		})

		It("delivers messages when IPChecker allows loopback", func() {
			drain := newBatchMockDrain(http.StatusOK)
			defer drain.Close()

			b := buildURLBinding(drain.URL, "test-app-id", "test-hostname")
			fakeChecker.CheckBlacklistReturns(nil)

			writer := syslog.NewHTTPSBatchWriter(
				b,
				syslog.NetworkTimeoutConfig{
					DialTimeout: 100 * time.Millisecond,
					IPChecker:   fakeChecker,
				},
				skipSSLTLSConfig,
				&metricsHelpers.SpyMetric{},
				c,
				syslog.WithSendInterval(10*time.Millisecond),
			)
			defer writer.Close()

			rw := writer.(syslog.InternalRetryWriter)
			rw.ConfigureRetry(func(i int) time.Duration { return 0 }, 0)

			Expect(writer.Write(env)).To(Succeed())
			Eventually(drain.getMessagesSize, 500*time.Millisecond).Should(Equal(1))
		})

		It("delivers messages when NetworkTimeoutConfig is zero-value (nil checker)", func() {
			drain := newBatchMockDrain(http.StatusOK)
			defer drain.Close()

			b := buildURLBinding(drain.URL, "test-app-id", "test-hostname")

			writer := syslog.NewHTTPSBatchWriter(
				b,
				syslog.NetworkTimeoutConfig{},
				skipSSLTLSConfig,
				&metricsHelpers.SpyMetric{},
				c,
				syslog.WithSendInterval(10*time.Millisecond),
			)
			defer writer.Close()

			rw := writer.(syslog.InternalRetryWriter)
			rw.ConfigureRetry(func(i int) time.Duration { return 0 }, 0)

			Expect(writer.Write(env)).To(Succeed())
			Eventually(drain.getMessagesSize, 500*time.Millisecond).Should(Equal(1))
		})

		It("does not follow 302 redirects to a second listener", func() {
			targetDrain := newBatchMockDrain(http.StatusOK)
			defer targetDrain.Close()

			redirectServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", targetDrain.URL)
				w.WriteHeader(http.StatusFound)
			}))
			defer redirectServer.Close()

			b := buildURLBinding(redirectServer.URL, "test-app-id", "test-hostname")

			writer := syslog.NewHTTPSBatchWriter(
				b,
				syslog.NetworkTimeoutConfig{},
				skipSSLTLSConfig,
				&metricsHelpers.SpyMetric{},
				c,
				syslog.WithSendInterval(10*time.Millisecond),
			)
			defer writer.Close()

			rw := writer.(syslog.InternalRetryWriter)
			rw.ConfigureRetry(func(i int) time.Duration { return 0 }, 0)

			batchWriter := writer.(*syslog.HTTPSBatchWriter)
			err := batchWriter.HTTPSWriter.Write(env)
			Expect(err).To(HaveOccurred())
			Expect(targetDrain.getRequestCount()).To(Equal(0))
		})
	})
})

func newBatchMockDrain(status int) *SpyDrain {
	drain := &SpyDrain{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		drain.mu.Lock()
		drain.requestCount++
		drain.mu.Unlock()
		body, err := io.ReadAll(r.Body)
		Expect(err).ToNot(HaveOccurred())
		defer r.Body.Close()

		message := &rfc5424.Message{}

		messages := bytes.SplitAfter(body, []byte("\n"))
		for _, raw := range messages {
			if bytes.Equal(raw, []byte("")) {
				continue
			}
			message = &rfc5424.Message{}
			err = message.UnmarshalBinary(raw)
			Expect(err).ToNot(HaveOccurred())
			drain.appendMessage(message)
			drain.appendHeader(r.Header)
		}
		w.WriteHeader(status)
	})
	server := httptest.NewTLSServer(handler)
	drain.Server = server
	return drain
}
