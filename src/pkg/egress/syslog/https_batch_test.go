package syslog_test

import (
	"bytes"
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"time"

	"code.cloudfoundry.org/go-loggregator/v10/rfc5424"
	"code.cloudfoundry.org/go-loggregator/v10/rpc/loggregator_v2"
	metricsHelpers "code.cloudfoundry.org/go-metric-registry/testhelpers"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/syslog"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var string_to_1024_chars = "saljdflajsdssdfsdfljkfkajafjajlköflkjöjaklgljksdjlakljkflkjweljklkwjejlkfekljwlkjefjklwjklsdajkljklwerlkaskldgjksakjekjwrjkljasdjkgfkljwejklrkjlklasdkjlsadjlfjlkadfljkajklsdfjklslkdfjkllkjasdjkflsdlakfjklasldfkjlasdjfkjlsadlfjklaljsafjlslkjawjklerkjljklasjkdfjklwerjljalsdjkflwerjlkwejlkarjklalkklfsdjlfhkjsdfkhsewhkjjasdjfkhwkejrkjahjefkhkasdjhfkashfkjwehfkksadfjaskfkhjdshjfhewkjhasdfjdajskfjwehkfajkankaskjdfasdjhfkkjhjjkasdfjhkjahksdf"

var _ = Describe("HTTPS_batch_testing", func() {
	var (
		netConf          syslog.NetworkTimeoutConfig
		skipSSLTLSConfig = &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec
		}
		c      = syslog.NewConverter()
		drain  *SpyDrain
		b      *syslog.URLBinding
		writer egress.WriteCloser
	)
	string_to_1024_chars += string_to_1024_chars

	BeforeEach(func() {
		drain = newBatchMockDrain(200)
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
		)
	})

	It("testing simple appending of one log", func() {
		env1 := buildLogEnvelope("APP", "1", "message 1", loggregator_v2.Log_OUT)
		Expect(writer.Write(env1)).To(Succeed())
		env2 := buildLogEnvelope("APP", "2", "message 2", loggregator_v2.Log_OUT)
		Expect(writer.Write(env2)).To(Succeed())
		time.Sleep(2 * time.Second)

		Expect(drain.messages).To(HaveLen(2))
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

	It("test early dispatch on high message load", func() {
		env1 := buildLogEnvelope("APP", "1", "string to get log to 1024 characters:"+string_to_1024_chars, loggregator_v2.Log_OUT)
		for i := 0; i < 300; i++ {
			writer.Write(env1)
		}
		time.Sleep(100 * time.Millisecond)
		Expect(drain.messages).To(HaveLen(256))
	})

	It("test batch dispatching with all logs in a given timeframe", func() {
		env1 := buildLogEnvelope("APP", "1", "string to get log to 1024 characters:"+string_to_1024_chars, loggregator_v2.Log_OUT)
		for i := 0; i < 10; i++ {
			writer.Write(env1)
			time.Sleep(99 * time.Millisecond)
		}
		Expect(drain.messages).To(HaveLen(0))
		time.Sleep(100 * time.Millisecond)
		Expect(drain.messages).To(HaveLen(10))
	})

	It("probabilistic test for race condition", func() {
		env1 := buildLogEnvelope("APP", "1", "string to get log to 1024 characters:"+string_to_1024_chars, loggregator_v2.Log_OUT)
		for i := 0; i < 10; i++ {
			writer.Write(env1)
			time.Sleep(99 * time.Millisecond)
		}
		time.Sleep(100 * time.Millisecond)
		Expect(drain.messages).To(HaveLen(10))
	})
})

func newBatchMockDrain(status int) *SpyDrain {
	drain := &SpyDrain{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		body, err := io.ReadAll(r.Body)
		Expect(err).ToNot(HaveOccurred())
		defer r.Body.Close()

		println(body)

		message := &rfc5424.Message{}

		messages := bytes.SplitAfter(body, []byte("\n"))
		for _, raw := range messages {
			if bytes.Equal(raw, []byte("")) {
				continue
			}
			message = &rfc5424.Message{}
			err = message.UnmarshalBinary(raw)
			Expect(err).ToNot(HaveOccurred())
			drain.messages = append(drain.messages, message)
			drain.headers = append(drain.headers, r.Header)
		}
		w.WriteHeader(status)
	})
	server := httptest.NewTLSServer(handler)
	drain.Server = server
	return drain
}
