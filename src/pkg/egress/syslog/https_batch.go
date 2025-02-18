package syslog

import (
	"bytes"
	"crypto/tls"
	"log"
	"time"

	"code.cloudfoundry.org/go-loggregator/v10/rpc/loggregator_v2"
	metrics "code.cloudfoundry.org/go-metric-registry"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress"
)

var DefaultBatchSize = 256 * 1024
var DefaultSendInterval = 1 * time.Second

type HTTPSBatchWriter struct {
	HTTPSWriter
	msgs         chan []byte
	batchSize    int
	sendInterval time.Duration
	egrMsgCount  float64
}

func NewHTTPSBatchWriter(
	binding *URLBinding,
	netConf NetworkTimeoutConfig,
	tlsConf *tls.Config,
	egressMetric metrics.Counter,
	c *Converter,
) egress.WriteCloser {
	client := httpClient(netConf, tlsConf)
	binding.URL.Scheme = "https" // reset the scheme for usage to a valid http scheme
	BatchWriter := &HTTPSBatchWriter{
		HTTPSWriter: HTTPSWriter{
			url:             binding.URL,
			appID:           binding.AppID,
			hostname:        binding.Hostname,
			client:          client,
			egressMetric:    egressMetric,
			syslogConverter: c,
		},
		batchSize:    DefaultBatchSize,
		sendInterval: DefaultSendInterval,
		egrMsgCount:  0,
		msgs:         make(chan []byte),
	}
	go BatchWriter.startSender()
	return BatchWriter
}

// Modified Write function
func (w *HTTPSBatchWriter) Write(env *loggregator_v2.Envelope) error {
	msgs, err := w.syslogConverter.ToRFC5424(env, w.hostname)
	if err != nil {
		log.Printf("Failed to parse syslog, dropping faulty message, err: %s", err)
		return nil
	}

	for _, msg := range msgs {
		//There is no correct way of implementing error based retries in the current architecture.
		//Retries for https-batching will be implemented at a later point in time.
		w.msgs <- msg
	}
	return nil
}

func (w *HTTPSBatchWriter) startSender() {
	var msgBatch bytes.Buffer
	var msgCount float64
	ticker := time.NewTicker(w.sendInterval)
	defer ticker.Stop()

	msgCount = 0

	sendBatch := func() {
		if msgBatch.Len() > 0 {
			w.sendHttpRequest(msgBatch.Bytes(), msgCount) //nolint:errcheck
			msgBatch.Reset()
			msgCount = 0
		}
	}

	for {
		select {
		case msg := <-w.msgs:
			_, buffer_err := msgBatch.Write(msg)
			if buffer_err != nil {
				log.Printf("Failed to write to buffer, dropping buffer of size %d , err: %s", msgBatch.Len(), buffer_err)
				msgBatch.Reset()
				msgCount = 0
			} else {
				msgCount++
				if msgBatch.Len() >= w.batchSize {
					sendBatch()
				}
			}
		case <-ticker.C:
			sendBatch()
		}
	}
}
