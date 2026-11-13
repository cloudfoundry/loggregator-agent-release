package syslog_test

import (
	"bytes"
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"time"

	"code.cloudfoundry.org/go-loggregator/v10/rfc5424"
	"code.cloudfoundry.org/go-loggregator/v10/rpc/loggregator_v2"
	metricsHelpers "code.cloudfoundry.org/go-metric-registry/testhelpers"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/syslog"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/valyala/fasthttp"
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

	Describe("response body limit", func() {
		It("succeeds when response body is exactly MaxResponseBodySize with Content-Length and reuses the connection", func() {
			server := newConnTrackingServer(func(w http.ResponseWriter, _ int) {
				w.Header().Set("Content-Length", strconv.Itoa(syslog.MaxResponseBodySize))
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(bytes.Repeat([]byte("a"), syslog.MaxResponseBodySize))
			})
			defer server.Close()

			b := buildURLBinding(server.URL, "test-app-id", "test-hostname")
			writer := syslog.NewHTTPSWriter(b, netConf, skipSSLTLSConfig, &metricsHelpers.SpyMetric{}, c)
			env := buildLogEnvelope("APP", "1", "just a test", loggregator_v2.Log_OUT)

			Expect(writer.Write(env)).To(Succeed())
			Expect(writer.Write(env)).To(Succeed())

			addrs := server.RemoteAddrs()
			Expect(addrs).To(HaveLen(2))
			// Same client addr => both requests went over one keep-alive connection.
			Expect(addrs[0]).To(Equal(addrs[1]),
				"expected the connection to be reused for a response of exactly MaxResponseBodySize, but a new one was opened")
		})

		It("errors when Content-Length is MaxResponseBodySize+1 and uses a new connection for the next write", func() {
			server := newConnTrackingServer(func(w http.ResponseWriter, requestNum int) {
				if requestNum == 1 {
					body := bytes.Repeat([]byte("a"), syslog.MaxResponseBodySize+1)
					w.Header().Set("Content-Length", strconv.Itoa(len(body)))
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write(body)
					return
				}
				w.WriteHeader(http.StatusOK)
			})
			defer server.Close()

			b := buildURLBinding(server.URL, "test-app-id", "test-hostname")
			writer := syslog.NewHTTPSWriter(b, netConf, skipSSLTLSConfig, &metricsHelpers.SpyMetric{}, c)
			env := buildLogEnvelope("APP", "1", "just a test", loggregator_v2.Log_OUT)

			Expect(writer.Write(env)).To(HaveOccurred())
			Expect(writer.Write(env)).To(Succeed())

			addrs := server.RemoteAddrs()
			Expect(addrs).To(HaveLen(2))
			// Different client addr => the first connection was closed, not reused via keep-alive.
			Expect(addrs[0]).ToNot(Equal(addrs[1]),
				"expected a new connection after an oversized response, but the old one was reused")
		})

		It("errors with ErrBodyTooLarge when a 10 MiB Content-Length is declared without sending a body", func() {
			release := make(chan struct{})
			server := newResponseServer(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Length", strconv.Itoa(10*1024*1024))
				w.WriteHeader(http.StatusOK)
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
				// Keep the handler alive without sending the promised body, so the
				// client is left waiting. Returning would make the server close the
				// connection on a short body, which is a different scenario.
				// Unblock when the client disconnects (r.Context() is cancelled; the
				// expected outcome) or on test cleanup via release, so server.Close()
				// does not hang waiting for this handler.
				select {
				case <-r.Context().Done():
				case <-release:
				}
			})
			defer func() {
				close(release)
				server.Close()
			}()

			b := buildURLBinding(server.URL, "test-app-id", "test-hostname")
			writer := syslog.NewHTTPSWriter(b, netConf, skipSSLTLSConfig, &metricsHelpers.SpyMetric{}, c)
			env := buildLogEnvelope("APP", "1", "just a test", loggregator_v2.Log_OUT)

			err := writer.Write(env)

			Expect(err).To(MatchError(fasthttp.ErrBodyTooLarge))
		})

		Describe("chunked transfer encoding", func() {
			It("succeeds when total chunked body is exactly MaxResponseBodySize", func() {
				server := newResponseServer(func(w http.ResponseWriter, r *http.Request) {
					flusher, ok := w.(http.Flusher)
					Expect(ok).To(BeTrue())

					w.WriteHeader(http.StatusOK)
					flusher.Flush()

					half := syslog.MaxResponseBodySize / 2
					_, _ = w.Write(bytes.Repeat([]byte("a"), half))
					flusher.Flush()
					_, _ = w.Write(bytes.Repeat([]byte("b"), syslog.MaxResponseBodySize-half))
					flusher.Flush()
				})
				defer server.Close()

				b := buildURLBinding(server.URL, "test-app-id", "test-hostname")
				writer := syslog.NewHTTPSWriter(b, netConf, skipSSLTLSConfig, &metricsHelpers.SpyMetric{}, c)
				env := buildLogEnvelope("APP", "1", "just a test", loggregator_v2.Log_OUT)

				Expect(writer.Write(env)).To(Succeed())
			})

			It("errors when total chunked body exceeds MaxResponseBodySize split across multiple chunks", func() {
				server := newResponseServer(func(w http.ResponseWriter, r *http.Request) {
					flusher, ok := w.(http.Flusher)
					Expect(ok).To(BeTrue())

					w.WriteHeader(http.StatusOK)
					flusher.Flush()

					_, _ = w.Write(bytes.Repeat([]byte("a"), syslog.MaxResponseBodySize))
					flusher.Flush()
					_, _ = w.Write([]byte("b"))
					flusher.Flush()
				})
				defer server.Close()

				b := buildURLBinding(server.URL, "test-app-id", "test-hostname")
				writer := syslog.NewHTTPSWriter(b, netConf, skipSSLTLSConfig, &metricsHelpers.SpyMetric{}, c)
				env := buildLogEnvelope("APP", "1", "just a test", loggregator_v2.Log_OUT)

				Expect(writer.Write(env)).To(HaveOccurred())
			})

			It("errors when receiving an endless chunked body", func() {
				release := make(chan struct{})
				server := newResponseServer(func(w http.ResponseWriter, r *http.Request) {
					flusher, ok := w.(http.Flusher)
					Expect(ok).To(BeTrue())

					w.WriteHeader(http.StatusOK)
					flusher.Flush()

					chunk := bytes.Repeat([]byte("c"), 512)
					for {
						select {
						case <-r.Context().Done():
							return
						case <-release:
							return
						default:
							if _, err := w.Write(chunk); err != nil {
								return
							}
							flusher.Flush()
						}
					}
				})
				defer func() {
					close(release)
					server.Close()
				}()

				b := buildURLBinding(server.URL, "test-app-id", "test-hostname")
				writer := syslog.NewHTTPSWriter(b, netConf, skipSSLTLSConfig, &metricsHelpers.SpyMetric{}, c)
				env := buildLogEnvelope("APP", "1", "just a test", loggregator_v2.Log_OUT)

				Expect(writer.Write(env)).To(MatchError(fasthttp.ErrBodyTooLarge))
			})
		})

		It("errors after the cap on an endless stream with no Content-Length and not chunked", func() {
			release := make(chan struct{})
			server := newResponseServer(func(w http.ResponseWriter, r *http.Request) {
				hj, ok := w.(http.Hijacker)
				Expect(ok).To(BeTrue())
				conn, bufrw, err := hj.Hijack()
				if err != nil {
					return
				}
				defer conn.Close()

				_, _ = bufrw.WriteString("HTTP/1.1 200 OK\r\nConnection: close\r\n\r\n")
				_ = bufrw.Flush()

				chunk := bytes.Repeat([]byte("d"), 512)
				for {
					select {
					case <-release:
						return
					default:
						_ = conn.SetWriteDeadline(time.Now().Add(500 * time.Millisecond))
						if _, err := conn.Write(chunk); err != nil {
							return
						}
					}
				}
			})
			defer func() {
				close(release)
				server.Close()
			}()

			b := buildURLBinding(server.URL, "test-app-id", "test-hostname")
			writer := syslog.NewHTTPSWriter(b, netConf, skipSSLTLSConfig, &metricsHelpers.SpyMetric{}, c)
			env := buildLogEnvelope("APP", "1", "just a test", loggregator_v2.Log_OUT)

			Expect(writer.Write(env)).To(MatchError(fasthttp.ErrBodyTooLarge))
		})
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

// newResponseServer starts a TLS server that fully consumes each request body
// before calling respond, so tests only need to describe the response under test.
func newResponseServer(respond http.HandlerFunc) *httptest.Server {
	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
		respond(w, r)
	}))
}

// connTrackingServer is a response server that records the client address
// (ip:ephemeral port, unique per TCP connection) of every request it serves.
// Comparing addresses across requests shows whether a connection was reused.
type connTrackingServer struct {
	*httptest.Server
	mu          sync.Mutex
	remoteAddrs []string
}

// newConnTrackingServer calls respond with the 1-based number of the request
// being served.
func newConnTrackingServer(respond func(w http.ResponseWriter, requestNum int)) *connTrackingServer {
	s := &connTrackingServer{}
	s.Server = newResponseServer(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.remoteAddrs = append(s.remoteAddrs, r.RemoteAddr)
		n := len(s.remoteAddrs)
		s.mu.Unlock()
		respond(w, n)
	})
	return s
}

func (s *connTrackingServer) RemoteAddrs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.remoteAddrs...)
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
