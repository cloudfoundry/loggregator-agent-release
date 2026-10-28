package syslog_test

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/url"
	"time"

	metricsHelpers "code.cloudfoundry.org/go-metric-registry/testhelpers"
	"code.cloudfoundry.org/loggregator-agent-release/src/internal/testhelper"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/binding/bindingfakes"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/syslog"

	"code.cloudfoundry.org/go-loggregator/v10/rpc/loggregator_v2"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("TLSWriter", func() {
	var (
		tlsConfig *tls.Config

		testCerts = testhelper.GenerateCerts("loggregatorCA")
		certFile  = testCerts.Cert("metron")
		keyFile   = testCerts.Key("metron")
		env       = buildLogEnvelope("APP", "2", "just a test", loggregator_v2.Log_OUT)
		netConf   = syslog.NetworkTimeoutConfig{
			WriteTimeout: time.Second,
		}

		egressCounter *metricsHelpers.SpyMetric
	)

	BeforeEach(func() {
		egressCounter = &metricsHelpers.SpyMetric{}

		tlsCert, err := tls.LoadX509KeyPair(certFile, keyFile)
		Expect(err).ToNot(HaveOccurred())

		tlsConfig = &tls.Config{
			Certificates:       []tls.Certificate{tlsCert},
			InsecureSkipVerify: true, //nolint:gosec
		}
	})

	It("speaks TLS", func() {
		listener, err := tls.Listen("tcp", "127.0.0.1:", tlsConfig)
		Expect(err).ToNot(HaveOccurred())

		url, _ := url.Parse(fmt.Sprintf("syslog-tls://%s", listener.Addr()))
		binding := &syslog.URLBinding{
			AppID:    "test-app-id",
			Hostname: "test-hostname",
			URL:      url,
		}
		logClient := testhelper.NewSpyLogClient()
		writer := syslog.NewTLSWriter(
			binding,
			netConf,
			&tls.Config{
				InsecureSkipVerify: true, //nolint:gosec
			},
			egressCounter,
			syslog.NewConverter(),
			logClient,
		)
		defer writer.Close()

		var conn net.Conn
		go func() {
			var err error
			conn, err = listener.Accept()
			Expect(err).ToNot(HaveOccurred())

			// Note: for some odd reason you have to do a read off of the TLS
			// connection before the dial will succeed. We should probably
			// investigate.
			empty := make([]byte, 0)
			_, err = conn.Read(empty)
			Expect(err).ToNot(HaveOccurred())
		}()

		Expect(writer.Write(env)).To(Succeed())

		buf := bufio.NewReader(conn)

		actual, err := buf.ReadString('\n')
		Expect(err).ToNot(HaveOccurred())

		expected := fmt.Sprintf(`118 <14>1 1970-01-01T00:00:00.012345+00:00 test-hostname test-app-id [APP/2] - [tags@47450 source_type="APP"] just a test` + "\n")
		Expect(actual).To(Equal(expected))

		By("emit an egress metric for each message")
		Expect(egressCounter.Value()).To(BeNumerically("==", 1))
	})

	Describe("IP blacklist enforcement", func() {
		It("fails write/dial with ErrBlockedAddress when IPChecker blocks loopback", func() {
			listener, err := tls.Listen("tcp", "127.0.0.1:", tlsConfig)
			Expect(err).ToNot(HaveOccurred())
			defer listener.Close()

			drainURL, _ := url.Parse(fmt.Sprintf("syslog-tls://%s", listener.Addr()))
			binding := &syslog.URLBinding{
				AppID:    "test-app-id",
				Hostname: "test-hostname",
				URL:      drainURL,
			}
			fakeChecker := &bindingfakes.FakeIPChecker{}
			fakeChecker.CheckBlacklistReturns(errors.New("blacklisted"))

			writer := syslog.NewTLSWriter(
				binding,
				syslog.NetworkTimeoutConfig{
					WriteTimeout: time.Second,
					DialTimeout:  100 * time.Millisecond,
					IPChecker:    fakeChecker,
				},
				&tls.Config{
					InsecureSkipVerify: true, //nolint:gosec
				},
				egressCounter,
				syslog.NewConverter(),
				testhelper.NewSpyLogClient(),
			)
			defer writer.Close()

			err = writer.Write(env)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, syslog.ErrBlockedAddress)).To(BeTrue())
		})

		It("connects and delivers the message when IPChecker allows loopback", func() {
			listener, err := tls.Listen("tcp", "127.0.0.1:", tlsConfig)
			Expect(err).ToNot(HaveOccurred())
			defer listener.Close()

			drainURL, _ := url.Parse(fmt.Sprintf("syslog-tls://%s", listener.Addr()))
			binding := &syslog.URLBinding{
				AppID:    "test-app-id",
				Hostname: "test-hostname",
				URL:      drainURL,
			}
			fakeChecker := &bindingfakes.FakeIPChecker{}
			fakeChecker.CheckBlacklistReturns(nil)

			writer := syslog.NewTLSWriter(
				binding,
				syslog.NetworkTimeoutConfig{
					WriteTimeout: time.Second,
					DialTimeout:  100 * time.Millisecond,
					IPChecker:    fakeChecker,
				},
				&tls.Config{
					InsecureSkipVerify: true, //nolint:gosec
				},
				egressCounter,
				syslog.NewConverter(),
				testhelper.NewSpyLogClient(),
			)
			defer writer.Close()

			var conn net.Conn
			go func() {
				var err error
				conn, err = listener.Accept()
				Expect(err).ToNot(HaveOccurred())

				empty := make([]byte, 0)
				_, err = conn.Read(empty)
				Expect(err).ToNot(HaveOccurred())
			}()

			Expect(writer.Write(env)).To(Succeed())

			buf := bufio.NewReader(conn)
			actual, err := buf.ReadString('\n')
			Expect(err).ToNot(HaveOccurred())

			expected := fmt.Sprintf(`118 <14>1 1970-01-01T00:00:00.012345+00:00 test-hostname test-app-id [APP/2] - [tags@47450 source_type="APP"] just a test` + "\n")
			Expect(actual).To(Equal(expected))
		})

		It("connects when NetworkTimeoutConfig is zero-value (nil checker)", func() {
			listener, err := tls.Listen("tcp", "127.0.0.1:", tlsConfig)
			Expect(err).ToNot(HaveOccurred())
			defer listener.Close()

			drainURL, _ := url.Parse(fmt.Sprintf("syslog-tls://%s", listener.Addr()))
			binding := &syslog.URLBinding{
				AppID:    "test-app-id",
				Hostname: "test-hostname",
				URL:      drainURL,
			}

			writer := syslog.NewTLSWriter(
				binding,
				syslog.NetworkTimeoutConfig{},
				&tls.Config{
					InsecureSkipVerify: true, //nolint:gosec
				},
				egressCounter,
				syslog.NewConverter(),
				testhelper.NewSpyLogClient(),
			)
			defer writer.Close()

			go func() {
				_ = writer.Write(env)
			}()

			conn, err := listener.Accept()
			Expect(err).ToNot(HaveOccurred())
			defer conn.Close()
		})
	})
})
