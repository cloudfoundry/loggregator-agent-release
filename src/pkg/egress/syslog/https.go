package syslog

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"code.cloudfoundry.org/go-loggregator/v10/rpc/loggregator_v2"
	metrics "code.cloudfoundry.org/go-metric-registry"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress"
	"github.com/valyala/fasthttp"
)

const BATCHSIZE = 256 * 1024

type HTTPSWriter struct {
	hostname        string
	appID           string
	url             *url.URL
	client          *fasthttp.Client
	egressMetric    metrics.Counter
	syslogConverter *Converter
	msgBatch        string
	batchSize       int
	sendInterval    time.Duration
	sendTimer       *time.Timer
}

func NewHTTPSWriter(
	binding *URLBinding,
	netConf NetworkTimeoutConfig,
	tlsConf *tls.Config,
	egressMetric metrics.Counter,
	c *Converter,
) egress.WriteCloser {

	client := httpClient(netConf, tlsConf)

	return &HTTPSWriter{
		url:             binding.URL,
		appID:           binding.AppID,
		hostname:        binding.Hostname,
		client:          client,
		egressMetric:    egressMetric,
		syslogConverter: c,
		msgBatch:        "",
		batchSize:       BATCHSIZE,
		sendInterval:    time.Second,
	}
}

func (w *HTTPSWriter) sendMsgBatch() error {
	req := fasthttp.AcquireRequest()
	req.SetRequestURI(w.url.String())
	req.Header.SetMethod("POST")
	req.Header.SetContentType("text/plain")
	req.SetBodyString(w.msgBatch)

	w.msgBatch = ""
	w.sendTimer = nil

	resp := fasthttp.AcquireResponse()

	err := w.client.Do(req, resp)
	if err != nil {
		return w.sanitizeError(w.url, err)
	}

	if resp.StatusCode() < 200 || resp.StatusCode() > 299 {
		return fmt.Errorf("syslog Writer: Post responded with %d status code", resp.StatusCode())
	}

	return nil
}

// Modified Write function
func (w *HTTPSWriter) Write(env *loggregator_v2.Envelope) error {
	msgs, err := w.syslogConverter.ToRFC5424(env, w.hostname)
	if err != nil {
		return err
	}

	for _, msg := range msgs {
		if len(w.msgBatch) == 0 {
			w.msgBatch = string(msg)
		} else {
			w.msgBatch += string(msg)
		}
		w.egressMetric.Add(1)
	}

	if w.sendTimer == nil {
		w.sendTimer = time.AfterFunc(w.sendInterval, func() {
			w.sendMsgBatch()
		})
	}

	if len(w.msgBatch) >= w.batchSize {
		w.sendTimer.Stop()
		err = w.sendMsgBatch()
		if err != nil {
			return err
		}
	}

	return nil
}

func (*HTTPSWriter) sanitizeError(u *url.URL, err error) error {
	if u == nil || u.User == nil {
		return err
	}

	if user := u.User.Username(); user != "" {
		err = errors.New(strings.Replace(err.Error(), user, "<REDACTED>", -1))
	}

	if p, ok := u.User.Password(); ok {
		err = errors.New(strings.Replace(err.Error(), p, "<REDACTED>", -1))
	}
	return err
}

func (*HTTPSWriter) Close() error {
	return nil
}

func httpClient(netConf NetworkTimeoutConfig, tlsConf *tls.Config) *fasthttp.Client {
	return &fasthttp.Client{
		MaxConnsPerHost:     5,
		MaxIdleConnDuration: 90 * time.Second,
		TLSConfig:           tlsConf,
		ReadTimeout:         20 * time.Second,
		WriteTimeout:        20 * time.Second,
	}
}
