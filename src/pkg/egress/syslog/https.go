package syslog

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	metrics "code.cloudfoundry.org/go-metric-registry"

	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	"code.cloudfoundry.org/loggregator-agent/pkg/egress"
	"github.com/valyala/fasthttp"
)

const batchSize = 250

var urlMatch *regexp.Regexp

type HTTPSWriter struct {
	hostname     string
	appID        string
	url          *url.URL
	client       *fasthttp.Client
	egressMetric metrics.Counter
	msgs         [][]byte
	timer        *time.Timer
	mutex        *sync.Mutex
}

func NewHTTPSWriter(
	binding *URLBinding,
	netConf NetworkTimeoutConfig,
	tlsConf *tls.Config,
	egressMetric metrics.Counter,
) egress.WriteCloser {

	urlMatch = regexp.MustCompile(`.*logs\..*\.logging\.cloud\.ibm\.com`)
	client := httpClient(netConf, tlsConf)
	newMutex := &sync.Mutex{}

	return &HTTPSWriter{
		url:          binding.URL,
		appID:        binding.AppID,
		hostname:     binding.Hostname,
		client:       client,
		egressMetric: egressMetric,
		mutex:        newMutex,
	}
}

func newFunc(w *HTTPSWriter) func() {
	return func() {
		sendNow(w)
	}
}

func sendNow(w *HTTPSWriter) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	w.timer = nil
	increment := 0
	var b []byte
	for _, msg2 := range w.msgs {
		b = append(b, msg2...)
		increment++
	}
	w.msgs = [][]byte{}
	req := fasthttp.AcquireRequest()
	req.SetRequestURI(w.url.String())
	req.Header.SetMethod("POST")
	req.Header.SetContentType("text/plain")
	req.SetBody(b)

	resp := fasthttp.AcquireResponse()

	err := w.client.Do(req, resp)
	if err != nil {
		fmt.Printf("Failed with error %v\n", w.sanitizeError(w.url, err))
	}
	if resp.StatusCode() < 200 || resp.StatusCode() > 299 {
		fmt.Printf("Syslog Writer: Post responded with %d status code\n", resp.StatusCode())
		return
	}

	w.egressMetric.Add(1)
}

func (w *HTTPSWriter) Write(env *loggregator_v2.Envelope) error {
	msgs, err := ToRFC5424(env, w.hostname)
	if err != nil {
		return err
	}

	for _, msg := range msgs {
		var b []byte
		var increment float64
		increment = 1
		if strings.Contains(w.url.String(), "splunkcloud") ||
			urlMatch.MatchString(w.url.String()) {
			w.mutex.Lock()
			w.msgs = append(w.msgs, msg)
			if w.timer == nil {
				w.timer = time.AfterFunc(5*time.Second, newFunc(w))
			}
			if len(w.msgs) == batchSize {
				// send
				for _, msg2 := range w.msgs {
					b = append(b, msg2...)
				}
				increment = batchSize
				w.msgs = [][]byte{}
			} else {
				w.mutex.Unlock()
				return nil
			}
			w.mutex.Unlock()
		} else {
			b = msg
		}
		w.mutex.Lock()
		if w.timer != nil {
			w.timer.Stop()
			w.timer = nil
		}
		w.mutex.Unlock()

		req := fasthttp.AcquireRequest()
		req.SetRequestURI(w.url.String())
		req.Header.SetMethod("POST")
		req.Header.SetContentType("text/plain")
		req.SetBody(b)

		resp := fasthttp.AcquireResponse()

		err := w.client.Do(req, resp)
		if err != nil {
			return w.sanitizeError(w.url, err)
		}

		if resp.StatusCode() < 200 || resp.StatusCode() > 299 {
			return fmt.Errorf("syslog Writer: Post responded with %d status code", resp.StatusCode())
		}

		w.egressMetric.Add(increment)
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
