package syslog

import (
	"bytes"
	"fmt"
	"log"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"code.cloudfoundry.org/go-loggregator/v10/rpc/loggregator_v2"
	metrics "code.cloudfoundry.org/go-metric-registry"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress"
)

// DialFunc represents a method for creating a connection, either TCP or TLS.
type DialFunc func(addr string) (net.Conn, error)

// TCPWriter represents a syslog writer that connects over unencrypted TCP.
// This writer is not meant to be used from multiple goroutines. The same
// goroutine that calls `.Write()` should be the one that calls `.Close()`.
type TCPWriter struct {
	url             *url.URL
	appID           string
	hostname        string
	dialFunc        DialFunc
	writeTimeout    time.Duration
	scheme          string
	conn            net.Conn
	syslogConverter *Converter

	egressMetric metrics.Counter
}

// NewTCPWriter creates a new TCP syslog writer.
func NewTCPWriter(
	binding *URLBinding,
	netConf NetworkTimeoutConfig,
	egressMetric metrics.Counter,
	c *Converter,
) egress.WriteCloser {
	dialer := &net.Dialer{
		Timeout:   netConf.DialTimeout,
		KeepAlive: netConf.Keepalive,
	}
	df := func(addr string) (net.Conn, error) {
		return dialer.Dial("tcp", addr)
	}

	w := &TCPWriter{
		url:             binding.URL,
		appID:           binding.AppID,
		hostname:        binding.Hostname,
		writeTimeout:    netConf.WriteTimeout,
		dialFunc:        df,
		scheme:          "syslog",
		egressMetric:    egressMetric,
		syslogConverter: c,
	}

	return w
}

// Write writes an envelope to the syslog drain connection.
func (w *TCPWriter) Write(env *loggregator_v2.Envelope) error {
	conn, err := w.connection()
	if err != nil {
		return err
	}

	msgs, err := w.syslogConverter.ToRFC5424(env, w.hostname)
	if err != nil {
		return err
	}

	for _, msg := range msgs {
		err = conn.SetWriteDeadline(time.Now().Add(w.writeTimeout))
		if err != nil {
			_ = w.Close()
			return err
		}

		_, err = conn.Write([]byte(strconv.Itoa(len(msg)) + " " + string(msg)))
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
		// todo send logs to app logstream
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
		// 128 is the max size, 3 for [] and /, truncate to fit
		// source type is almost certainly very small
		if len(sourceType)+len(sourceInstance)+3 > 128 {
			sourceInstance = sourceInstance[:(128 - len(sourceType) - 3)]
		}
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
