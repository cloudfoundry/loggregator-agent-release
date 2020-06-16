package syslog

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	metrics "code.cloudfoundry.org/go-metric-registry"
	"code.cloudfoundry.org/loggregator-agent/pkg/egress"
	"github.com/valyala/fasthttp"
)

type HTTPSWriter struct {
	hostname     string
	appID        string
	url          *url.URL
	client       *fasthttp.Client
	egressMetric metrics.Counter
}

func NewHTTPSWriter(
	binding *URLBinding,
	netConf NetworkTimeoutConfig,
	tlsConf *tls.Config,
	egressMetric metrics.Counter,
) egress.WriteCloser {

	client := httpClient(netConf, tlsConf)

	return &HTTPSWriter{
		url:          binding.URL,
		appID:        binding.AppID,
		hostname:     binding.Hostname,
		client:       client,
		egressMetric: egressMetric,
	}
}

func (w *HTTPSWriter) Write(env *loggregator_v2.Envelope) error {
	msgs, err := NewConverter().ToRFC5424(env, w.hostname)
	if err != nil {
		return err
	}

	for _, msg := range msgs {
		req := fasthttp.AcquireRequest()
		req.SetRequestURI(w.url.String())
		req.Header.SetMethod("POST")
		req.Header.SetContentType("text/plain")
		req.SetBody(msg)

		resp := fasthttp.AcquireResponse()

		err := w.client.Do(req, resp)
		if err != nil {
			return w.sanitizeError(w.url, err)
		}

		if resp.StatusCode() < 200 || resp.StatusCode() > 299 {
			return fmt.Errorf("syslog Writer: Post responded with %d status code", resp.StatusCode())
		}

		w.egressMetric.Add(1)
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
