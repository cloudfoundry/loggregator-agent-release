package syslog

import (
	"bytes"
	"crypto/tls"
	"time"

	"code.cloudfoundry.org/go-loggregator/v9/rpc/loggregator_v2"
	metrics "code.cloudfoundry.org/go-metric-registry"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress"
)

const BATCHSIZE = 256 * 1024

type HTTPSBatchWriter struct {
	HTTPSWriter
	msgBatch     bytes.Buffer
	batchSize    int
	sendInterval time.Duration
	sendTimer    TriggerTimer
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
	return &HTTPSBatchWriter{
		HTTPSWriter: HTTPSWriter{
			url:             binding.URL,
			appID:           binding.AppID,
			hostname:        binding.Hostname,
			client:          client,
			egressMetric:    egressMetric,
			syslogConverter: c,
		},
		batchSize:    BATCHSIZE,
		sendInterval: time.Second,
		egrMsgCount:  0,
	}
}

func (w *HTTPSBatchWriter) sendMsgBatch() error {
	currentEgrCount := w.egrMsgCount
	currentMsg := w.msgBatch.Bytes()

	w.egrMsgCount = 0
	w.msgBatch.Reset()

	return w.sendHttpRequest(currentMsg, currentEgrCount)
}

// Modified Write function
func (w *HTTPSBatchWriter) Write(env *loggregator_v2.Envelope) error {
	msgs, err := w.syslogConverter.ToRFC5424(env, w.hostname)
	if err != nil {
		return err
	}

	for _, msg := range msgs {
		w.msgBatch.Write(msg)
		w.egrMsgCount += 1
		w.startAndTriggerSend()
	}
	return nil
}

// TODO: Error back propagation. Errors are not looked at further down the call chain
func (w *HTTPSBatchWriter) startAndTriggerSend() {
	if !w.sendTimer.Running() {
		w.sendTimer.Start(w.sendInterval, func() {
			w.sendMsgBatch()
		})
	}
	if w.msgBatch.Len() >= w.batchSize {
		w.sendTimer.Trigger()
	}
}

type TriggerTimer struct {
	trigger chan int
	running bool
}

type Timer interface {
	Start(d time.Duration, f func())
}

func NewTriggerTimer() Timer {
	return &TriggerTimer{
		running: false,
	}
}

func (t *TriggerTimer) Start(d time.Duration, f func()) {
	t.running = true
	for {
		timer := time.NewTimer(d)
		select {
		case <-timer.C:
		case <-t.trigger:
			f()
			t.running = false
		}
	}
}

func (t *TriggerTimer) Trigger() {
	t.trigger <- 1
}

func (t *TriggerTimer) Running() bool {
	return t.running
}
