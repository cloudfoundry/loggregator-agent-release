package collector

import (
	"reflect"

	"code.cloudfoundry.org/loggregator-agent-release/src/cmd/otel-collector-manager/app"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

//counterfeiter:generate . Getter
type Getter interface {
	Get() (app.ExporterConfig, error)
}

type ChangeGetter struct {
	g  Getter
	hc bool
	lc app.ExporterConfig
}

func NewChangeGetter(g Getter) *ChangeGetter {
	return &ChangeGetter{g: g}
}

func (c *ChangeGetter) Get() (app.ExporterConfig, error) {
	cfg, err := c.g.Get()
	if err == nil {
		c.hc = !reflect.DeepEqual(c.lc, cfg)
		c.lc = cfg
	}
	return cfg, err
}

func (g *ChangeGetter) Changed() bool {
	return g.hc
}
