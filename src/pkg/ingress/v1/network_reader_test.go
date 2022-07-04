package v1_test

import (
	"net"
	"strconv"
	"sync"

	metricsHelpers "code.cloudfoundry.org/go-metric-registry/testhelpers"
	ingress "code.cloudfoundry.org/loggregator-agent-release/src/pkg/ingress/v1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func randomPort() int {
	addr, err := net.ResolveUDPAddr("udp", "1.2.3.4:1")
	Expect(err).NotTo(HaveOccurred())
	conn, err := net.DialUDP("udp", nil, addr)
	Expect(err).NotTo(HaveOccurred())
	defer conn.Close()
	_, addrPort, err := net.SplitHostPort(conn.LocalAddr().String())
	Expect(err).NotTo(HaveOccurred())
	port, err := strconv.Atoi(addrPort)
	Expect(err).NotTo(HaveOccurred())
	return port
}

var _ = Describe("NetworkReader", func() {
	var (
		reader        *ingress.NetworkReader
		readerStopped chan struct{}
		writer        MockByteArrayWriter
		port          int
		address       string
		metricClient  *metricsHelpers.SpyMetricsRegistry
	)

	BeforeEach(func() {
		port = randomPort() + GinkgoParallelProcess()
		address = net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
		writer = MockByteArrayWriter{}
		metricClient = metricsHelpers.NewMetricsRegistry()
		var err error
		reader, err = ingress.NewNetworkReader(address, &writer, metricClient)
		Expect(err).NotTo(HaveOccurred())
		readerStopped = make(chan struct{})
	})

	Context("with a reader running", func() {
		BeforeEach(func() {
			go reader.StartWriting()
			go func() {
				reader.StartReading()
				close(readerStopped)
			}()
		})

		AfterEach(func() {
			reader.Stop()
			<-readerStopped
		})

		It("sends data received on UDP socket to its writer", func() {
			expectedData := "Some Data"

			connection, err := net.Dial("udp", address)

			f := func() int {
				_, err = connection.Write([]byte(expectedData))
				Expect(err).NotTo(HaveOccurred())

				return len(writer.Data())
			}

			Eventually(f).ShouldNot(BeZero())
			data := string(writer.Data()[0])
			Expect(data).To(Equal(expectedData))

			metric := metricClient.GetMetric("ingress", map[string]string{"metric_version": "1.0"})
			Expect(metric.Value()).ToNot(BeZero())
		})
	})
})

type MockByteArrayWriter struct {
	data [][]byte
	lock sync.RWMutex
}

func (m *MockByteArrayWriter) Write(p []byte) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.data = append(m.data, p)
}

func (m *MockByteArrayWriter) Data() [][]byte {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return m.data
}

func (m *MockByteArrayWriter) Weight() int {
	return 0
}
