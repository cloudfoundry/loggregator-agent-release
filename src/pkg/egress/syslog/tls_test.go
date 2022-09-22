package syslog_test

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"time"

	metricsHelpers "code.cloudfoundry.org/go-metric-registry/testhelpers"
	"code.cloudfoundry.org/loggregator-agent-release/src/internal/testhelper"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/syslog"

	"code.cloudfoundry.org/go-loggregator/v9/rpc/loggregator_v2"

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
		writer := syslog.NewTLSWriter(
			binding,
			netConf,
			&tls.Config{
				InsecureSkipVerify: true, //nolint:gosec
			},
			egressCounter,
			syslog.NewConverter(),
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
})
