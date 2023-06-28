package collector

import (
	"reflect"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

//counterfeiter:generate . Getter
type Getter interface {
	GetAggregateMetric() (map[string]any, error)
}

type ChangeGetter struct {
	g  Getter
	hc bool
	lc map[string]any
}

func NewChangeGetter(g Getter) *ChangeGetter {
	return &ChangeGetter{g: g}
}

func (c *ChangeGetter) GetAggregateMetric() (map[string]any, error) {
	cfg, err := c.g.GetAggregateMetric()
	if err == nil {
		c.hc = !reflect.DeepEqual(c.lc, cfg)
		c.lc = cfg
	}
	return cfg, err
}

func (g *ChangeGetter) Changed() bool {
	return g.hc
}
