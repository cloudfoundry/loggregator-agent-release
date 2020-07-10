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
	URL          *url.URL
}

// Scheme is a convenience wrapper around the *url.URL Scheme field
func (u *URLBinding) Scheme() string {
	return u.URL.Scheme
}

func buildBinding(c context.Context, b Binding) (*URLBinding, error) {
	url, err := url.Parse(b.Drain)
	if err != nil {
		return nil, err
	}

	u := &URLBinding{
		AppID:        b.AppId,
		OmitMetadata: b.OmitMetadata,
		URL:          url,
		Hostname:     b.Hostname,
		Context:      c,
	}

	return u, nil
}
