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
	sendTimer    *TriggerTimer
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
	if w.sendTimer == nil || !w.sendTimer.Running() {
		w.sendTimer = NewTriggerTimer(w.sendInterval, func() {
			w.sendMsgBatch()
		})
	}
	if w.msgBatch.Len() >= w.batchSize {
		w.sendTimer.Trigger()
	}
}

type TriggerTimer struct {
	triggered bool
	execFunc  func()
}

func NewTriggerTimer(d time.Duration, f func()) *TriggerTimer {
	timer := &TriggerTimer{
		triggered: false,
		execFunc:  f,
	}
	timer.initWait(d)

	return timer
}

func (t *TriggerTimer) initWait(duration time.Duration) {
	timer := time.NewTimer(duration)
	<-timer.C
	if !t.triggered {
		t.execFunc()
	}

}

func (t *TriggerTimer) Trigger() {
	t.triggered = true
	t.execFunc()
}

func (t *TriggerTimer) Running() bool {
	return !t.triggered
}
