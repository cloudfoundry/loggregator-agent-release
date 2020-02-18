package cache

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/binding"
)

var pathTemplate = "%s/bindings"

type httpGetter interface {
	Get(string) (*http.Response, error)
}

type CacheClient struct {
	cacheAddr string
	h         httpGetter
}

func NewClient(cacheAddr string, h httpGetter) *CacheClient {
	return &CacheClient{
		cacheAddr: cacheAddr,
		h:         h,
	}
}

func (c *CacheClient) Get() ([]binding.Binding, error) {
	var bindings []binding.Binding
	resp, err := c.h.Get(fmt.Sprintf(pathTemplate, c.cacheAddr))
	if err != nil {
		return nil, err
	}
	defer func() {
		io.Copy(ioutil.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected http response from binding cache: %d", resp.StatusCode)
	}

	err = json.NewDecoder(resp.Body).Decode(&bindings)
	if err != nil {
		return nil, err
	}

	return bindings, nil
}
