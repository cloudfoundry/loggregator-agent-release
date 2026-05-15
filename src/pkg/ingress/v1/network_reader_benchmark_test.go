package v1_test

import (
	"net"
	"strconv"
	"testing"

	metricsHelpers "code.cloudfoundry.org/go-metric-registry/testhelpers"
	ingress "code.cloudfoundry.org/loggregator-agent-release/src/pkg/ingress/v1"
)

type benchmarkWriter struct {
	ch chan struct{}
}

func (w *benchmarkWriter) Write(p []byte) {
	w.ch <- struct{}{}
}

func getFreePort(b *testing.B) int {
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		b.Fatal(err)
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		b.Fatal(err)
	}
	defer conn.Close()
	_, addrPort, err := net.SplitHostPort(conn.LocalAddr().String())
	if err != nil {
		b.Fatal(err)
	}
	port, err := strconv.Atoi(addrPort)
	if err != nil {
		b.Fatal(err)
	}
	return port
}

func benchmarkNetworkReaderWithSize(b *testing.B, size int) {
	port := getFreePort(b)
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	
	metricClient := metricsHelpers.NewMetricsRegistry()
	writer := &benchmarkWriter{
		ch: make(chan struct{}, 1),
	}

	reader, err := ingress.NewNetworkReader(address, writer, metricClient)
	if err != nil {
		b.Fatalf("failed to create network reader: %s", err)
	}
	defer reader.Stop()

	go reader.StartReading()
	go reader.StartWriting()

	connection, err := net.Dial("udp", address)
	if err != nil {
		b.Fatalf("failed to dial: %s", err)
	}
	defer connection.Close()

	packet := make([]byte, size)
	for i := range packet {
		packet[i] = 'a'
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := connection.Write(packet)
		if err != nil {
			b.Fatalf("failed to write: %s", err)
		}
		<-writer.ch
	}
}

func BenchmarkNetworkReader_8KB(b *testing.B) {
	benchmarkNetworkReaderWithSize(b, 8192)
}

func BenchmarkNetworkReader_16KB(b *testing.B) {
	benchmarkNetworkReaderWithSize(b, 16384)
}

func BenchmarkNetworkReader_32KB(b *testing.B) {
	benchmarkNetworkReaderWithSize(b, 32768)
}

func BenchmarkNetworkReader_48KB(b *testing.B) {
	benchmarkNetworkReaderWithSize(b, 49152)
}

func BenchmarkNetworkReader_64KB(b *testing.B) {
	benchmarkNetworkReaderWithSize(b, 65507)  //65535 is the max UDP size, but we need to leave room for the headers: 20 for IPv4 and 8 for UDP
}
