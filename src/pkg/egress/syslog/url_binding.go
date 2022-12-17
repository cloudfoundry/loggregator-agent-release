package syslog

// URLBinding associates a particular application with a syslog URL. The
import (
	"context"
	"net/url"
)

// application is identified by AppID and Hostname. The syslog URL is
// identified by URL.
type URLBinding struct {
	Context      context.Context
	AppID        string
	Hostname     string
	OmitMetadata bool
	InternalTls  bool
	URL          *url.URL
	PrivateKey   []byte
	Certificate  []byte
	CA           []byte
}

// Scheme is a convenience wrapper around the *url.URL Scheme field
func (u *URLBinding) Scheme() string {
	return u.URL.Scheme
}

func buildBinding(c context.Context, b Binding) (*URLBinding, error) {
	url, err := url.Parse(b.Drain.Url)
	if err != nil {
		return nil, err
	}

	u := &URLBinding{
		AppID:        b.AppId,
		OmitMetadata: b.OmitMetadata,
		InternalTls:  b.InternalTls,
		URL:          url,
		Hostname:     b.Hostname,
		Context:      c,
		PrivateKey:   []byte(b.Drain.Credentials.Key),
		Certificate:  []byte(b.Drain.Credentials.Cert),
		CA:           []byte(b.Drain.Credentials.CA),
	}

	return u, nil
}
