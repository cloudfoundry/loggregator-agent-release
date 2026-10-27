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
			var (
				mu          sync.Mutex
				remoteAddrs []string
			)
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				remoteAddrs = append(remoteAddrs, r.RemoteAddr)
				mu.Unlock()

				_, _ = io.Copy(io.Discard, r.Body)
				_ = r.Body.Close()

				w.Header().Set("Content-Length", strconv.Itoa(syslog.MaxResponseBodySize))
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(bytes.Repeat([]byte("a"), syslog.MaxResponseBodySize))
			}))
			defer server.Close()

			b := buildURLBinding(server.URL, "test-app-id", "test-hostname")
			writer := syslog.NewHTTPSWriter(b, netConf, skipSSLTLSConfig, &metricsHelpers.SpyMetric{}, c)
			env := buildLogEnvelope("APP", "1", "just a test", loggregator_v2.Log_OUT)

			Expect(writer.Write(env)).To(Succeed())
			Expect(writer.Write(env)).To(Succeed())

			mu.Lock()
			defer mu.Unlock()
			Expect(remoteAddrs).To(HaveLen(2))
			Expect(remoteAddrs[0]).To(Equal(remoteAddrs[1]))
		})

		It("errors when Content-Length is MaxResponseBodySize+1 and uses a new connection for the next write", func() {
			var (
				mu          sync.Mutex
				remoteAddrs []string
			)
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				remoteAddrs = append(remoteAddrs, r.RemoteAddr)
				first := len(remoteAddrs) == 1
				mu.Unlock()

				_, _ = io.Copy(io.Discard, r.Body)
				_ = r.Body.Close()

				if first {
					body := bytes.Repeat([]byte("a"), syslog.MaxResponseBodySize+1)
					w.Header().Set("Content-Length", strconv.Itoa(len(body)))
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write(body)
				} else {
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("ok"))
				}
			}))
			defer server.Close()

			b := buildURLBinding(server.URL, "test-app-id", "test-hostname")
			writer := syslog.NewHTTPSWriter(b, netConf, skipSSLTLSConfig, &metricsHelpers.SpyMetric{}, c)
			env := buildLogEnvelope("APP", "1", "just a test", loggregator_v2.Log_OUT)

			Expect(writer.Write(env)).To(HaveOccurred())
			Expect(writer.Write(env)).To(Succeed())

			mu.Lock()
			defer mu.Unlock()
			Expect(remoteAddrs).To(HaveLen(2))
			Expect(remoteAddrs[0]).ToNot(Equal(remoteAddrs[1]))
		})

		It("errors quickly when huge Content-Length (10 MiB) is declared without sending a body", func() {
			release := make(chan struct{})
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				_ = r.Body.Close()

				w.Header().Set("Content-Length", strconv.Itoa(10*1024*1024))
				w.WriteHeader(http.StatusOK)
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
				select {
				case <-r.Context().Done():
				case <-release:
				}
			}))
			defer func() {
				close(release)
				server.Close()
			}()

			b := buildURLBinding(server.URL, "test-app-id", "test-hostname")
			writer := syslog.NewHTTPSWriter(b, netConf, skipSSLTLSConfig, &metricsHelpers.SpyMetric{}, c)
			env := buildLogEnvelope("APP", "1", "just a test", loggregator_v2.Log_OUT)

			start := time.Now()
			err := writer.Write(env)
			elapsed := time.Since(start)

			Expect(err).To(HaveOccurred())
			Expect(elapsed).To(BeNumerically("<", 2*time.Second))
		})

		Describe("chunked transfer encoding", func() {
			It("succeeds when total chunked body is exactly MaxResponseBodySize", func() {
				server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_, _ = io.Copy(io.Discard, r.Body)
					_ = r.Body.Close()

					flusher, ok := w.(http.Flusher)
					Expect(ok).To(BeTrue())

					w.WriteHeader(http.StatusOK)
					flusher.Flush()

					half := syslog.MaxResponseBodySize / 2
					_, _ = w.Write(bytes.Repeat([]byte("a"), half))
					flusher.Flush()
					_, _ = w.Write(bytes.Repeat([]byte("b"), syslog.MaxResponseBodySize-half))
					flusher.Flush()
				}))
				defer server.Close()

				b := buildURLBinding(server.URL, "test-app-id", "test-hostname")
				writer := syslog.NewHTTPSWriter(b, netConf, skipSSLTLSConfig, &metricsHelpers.SpyMetric{}, c)
				env := buildLogEnvelope("APP", "1", "just a test", loggregator_v2.Log_OUT)

				Expect(writer.Write(env)).To(Succeed())
			})

			It("errors when total chunked body exceeds MaxResponseBodySize split across multiple chunks", func() {
				server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_, _ = io.Copy(io.Discard, r.Body)
					_ = r.Body.Close()

					flusher, ok := w.(http.Flusher)
					Expect(ok).To(BeTrue())

					w.WriteHeader(http.StatusOK)
					flusher.Flush()

					_, _ = w.Write(bytes.Repeat([]byte("a"), syslog.MaxResponseBodySize))
					flusher.Flush()
					_, _ = w.Write([]byte("b"))
					flusher.Flush()
				}))
				defer server.Close()

				b := buildURLBinding(server.URL, "test-app-id", "test-hostname")
				writer := syslog.NewHTTPSWriter(b, netConf, skipSSLTLSConfig, &metricsHelpers.SpyMetric{}, c)
				env := buildLogEnvelope("APP", "1", "just a test", loggregator_v2.Log_OUT)

				Expect(writer.Write(env)).To(HaveOccurred())
			})

			It("errors when receiving an endless chunked body", func() {
				release := make(chan struct{})
				server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_, _ = io.Copy(io.Discard, r.Body)
					_ = r.Body.Close()

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
				}))
				defer func() {
					close(release)
					server.Close()
				}()

				b := buildURLBinding(server.URL, "test-app-id", "test-hostname")
				writer := syslog.NewHTTPSWriter(b, netConf, skipSSLTLSConfig, &metricsHelpers.SpyMetric{}, c)
				env := buildLogEnvelope("APP", "1", "just a test", loggregator_v2.Log_OUT)

				start := time.Now()
				Expect(writer.Write(env)).To(HaveOccurred())
				Expect(time.Since(start)).To(BeNumerically("<", 2*time.Second))
			})
		})

		It("errors after the cap on an endless stream with no Content-Length and not chunked", func() {
			release := make(chan struct{})
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				_ = r.Body.Close()

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
			}))
			defer func() {
				close(release)
				server.Close()
			}()

			b := buildURLBinding(server.URL, "test-app-id", "test-hostname")
			writer := syslog.NewHTTPSWriter(b, netConf, skipSSLTLSConfig, &metricsHelpers.SpyMetric{}, c)
			env := buildLogEnvelope("APP", "1", "just a test", loggregator_v2.Log_OUT)

			start := time.Now()
			Expect(writer.Write(env)).To(HaveOccurred())
			Expect(time.Since(start)).To(BeNumerically("<", 2*time.Second))
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
