package cache

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/binding"
)

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
	err := c.get("v2/bindings", &bindings)
	return bindings, err
}

func (c *CacheClient) LegacyGet() ([]binding.LegacyBinding, error) {
	var bindings []binding.LegacyBinding
	err := c.get("bindings", &bindings)
	return bindings, err
}

func (c *CacheClient) GetAggregate() ([]binding.Binding, error) {
	var bindings []binding.Binding
	err := c.get("v2/aggregate", &bindings)
	return bindings, err
}

func (c *CacheClient) GetLegacyAggregate() ([]binding.LegacyBinding, error) {
	var bindings []binding.LegacyBinding
	err := c.get("aggregate", &bindings)
	return bindings, err
}

func (c *CacheClient) GetAggregateMetric() (map[string]any, error) {
	var bindings map[string]any
	err := c.get("v2/aggregatemetric", &bindings)
	return bindings, err
}

func (c *CacheClient) get(path string, result any) error {
	resp, err := c.h.Get(fmt.Sprintf("%s/"+path, c.cacheAddr))
	if err != nil {
		return err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected http response from binding cache: %d", resp.StatusCode)
	}

	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return err
	}

	return nil
}
