package v1

import (
	"log"
	"net"

	metrics "code.cloudfoundry.org/go-metric-registry"

	gendiodes "code.cloudfoundry.org/go-diodes"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/diodes"
)

type ByteArrayWriter interface {
	Write(message []byte)
}

type MetricClient interface {
	NewCounter(name, helpText string, opts ...metrics.MetricOption) metrics.Counter
}

type NetworkReader struct {
	connection net.PacketConn
	writer     ByteArrayWriter
	rxMsgCount func(uint64)
	buffer     *diodes.OneToOne
}

func NewNetworkReader(
	address string,
	writer ByteArrayWriter,
	m MetricClient,
) (*NetworkReader, error) {
	connection, err := net.ListenPacket("udp4", address)
	if err != nil {
		return nil, err
	}
	log.Printf("udp bound to: %s", connection.LocalAddr())
	rxErrCount := m.NewCounter(
		"dropped",
		"Total number of dropped envelopes.",
		metrics.WithMetricLabels(map[string]string{"direction": "all", "metric_version": "1.0"}),
	)
	rxMsgCount := m.NewCounter(
		"ingress",
		"Total number of envelopes ingressed by the agent.",
		metrics.WithMetricLabels(map[string]string{"metric_version": "1.0"}),
	)

	return &NetworkReader{
		connection: connection,
		rxMsgCount: func(i uint64) { rxMsgCount.Add(float64(i)) },
		writer:     writer,
		buffer: diodes.NewOneToOne(10000, gendiodes.AlertFunc(func(missed int) {
			log.Printf("network reader dropped messages %d", missed)
			rxErrCount.Add(float64(missed))
		})),
	}, nil
}

func (nr *NetworkReader) StartReading() {
	readBuffer := make([]byte, 65535) //buffer with size = max theoretical UDP size
	for {
		readCount, _, err := nr.connection.ReadFrom(readBuffer)
		if err != nil {
			log.Printf("Error while reading: %s", err)
			return
		}
		readData := make([]byte, readCount)
		copy(readData, readBuffer[:readCount])

		nr.buffer.Set(readData)
	}
}

func (nr *NetworkReader) StartWriting() {
	for {
		data := nr.buffer.Next()
		nr.rxMsgCount(1)
		nr.writer.Write(data)
	}
}

func (nr *NetworkReader) Stop() {
	nr.connection.Close()
}
