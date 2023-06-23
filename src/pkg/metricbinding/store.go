package metricbinding

import (
	"os"

	"gopkg.in/yaml.v3"
)

type OtelExporterConfig map[string]any

type AggregateStore struct {
	c OtelExporterConfig
}

func NewAggregateStore(drainFileName string) (*AggregateStore, error) {
	b, err := os.ReadFile(drainFileName)
	if err != nil {
		return nil, err
	}

	s := &AggregateStore{c: OtelExporterConfig{}}
	err = yaml.Unmarshal(b, &s.c)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (s *AggregateStore) Get() OtelExporterConfig {
	return s.c
}
