package syslog

import (
	"bytes"
	"code.cloudfoundry.org/loggregator-agent/pkg/metrics"
	"fmt"
	"log"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	"code.cloudfoundry.org/loggregator-agent/pkg/egress"
)

// gaugeStructuredDataID contains the registered enterprise ID for the Cloud
// Foundry Foundation.
// See: https://www.iana.org/assignments/enterprise-numbers/enterprise-numbers
const (
	gaugeStructuredDataID   = "gauge@47450"
	counterStructuredDataID = "counter@47450"
)

// DialFunc represents a method for creating a connection, either TCP or TLS.
type DialFunc func(addr string) (net.Conn, error)

// TCPWriter represents a syslog writer that connects over unencrypted TCP.
// This writer is not meant to be used from multiple goroutines. The same
// goroutine that calls `.Write()` should be the one that calls `.Close()`.
type TCPWriter struct {
	url          *url.URL
	appID        string
	hostname     string
	dialFunc     DialFunc
	writeTimeout time.Duration
	scheme       string
	conn         net.Conn

	egressMetric metrics.Counter
}

// NewTCPWriter creates a new TCP syslog writer.
func NewTCPWriter(
	binding *URLBinding,
	netConf NetworkTimeoutConfig,
	skipCertVerify bool,
	egressMetric metrics.Counter,
) egress.WriteCloser {
	dialer := &net.Dialer{
		Timeout:   netConf.DialTimeout,
		KeepAlive: netConf.Keepalive,
	}
	df := func(addr string) (net.Conn, error) {
		return dialer.Dial("tcp", addr)
	}

	w := &TCPWriter{
		url:          binding.URL,
		appID:        binding.AppID,
		hostname:     binding.Hostname,
		writeTimeout: netConf.WriteTimeout,
		dialFunc:     df,
		scheme:       "syslog",
		egressMetric: egressMetric,
	}

	return w
}

// Write writes an envelope to the syslog drain connection.
func (w *TCPWriter) Write(env *loggregator_v2.Envelope) error {
	conn, err := w.connection()
	if err != nil {
		return err
	}

	msgs, err := ToRFC5424(env, w.hostname, w.appID)
	if err != nil {
		return err
	}

	for _, msg := range msgs {
		conn.SetWriteDeadline(time.Now().Add(w.writeTimeout))
		_, err = conn.Write([]byte(strconv.Itoa(len(msg)) + " "))
		if err != nil {
			_ = w.Close()
			return err
		}

		_, err = conn.Write(msg)
		if err != nil {
			_ = w.Close()
			return err
		}

		w.egressMetric.Add(1)
	}

	return nil
}

func (w *TCPWriter) connection() (net.Conn, error) {
	if w.conn == nil {
		return w.connect()
	}
	return w.conn, nil
}

func (w *TCPWriter) connect() (net.Conn, error) {
	conn, err := w.dialFunc(w.url.Host)
	if err != nil {
		return nil, err
	}
	w.conn = conn

	log.Printf("created conn to syslog drain: %s", w.url.Host)

	return conn, nil
}

// Close tears down any active connections to the drain and prevents reconnect.
func (w *TCPWriter) Close() error {
	if w.conn != nil {
		err := w.conn.Close()
		w.conn = nil

		return err
	}

	return nil
}

func removeNulls(msg []byte) []byte {
	return bytes.Replace(msg, []byte{0}, nil, -1)
}

func appendNewline(msg []byte) []byte {
	if !bytes.HasSuffix(msg, []byte("\n")) {
		msg = append(msg, byte('\n'))
	}
	return msg
}

func generateProcessID(sourceType, sourceInstance string) string {
	sourceType = strings.ToUpper(sourceType)
	if sourceInstance != "" {
		tmp := make([]byte, 0, 3+len(sourceType)+len(sourceInstance))
		tmp = append(tmp, '[')
		tmp = append(tmp, []byte(strings.Replace(sourceType, " ", "-", -1))...)
		tmp = append(tmp, '/')
		tmp = append(tmp, []byte(sourceInstance)...)
		tmp = append(tmp, ']')

		return string(tmp)
	}

	return fmt.Sprintf("[%s]", sourceType)
}
