package app

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

type ExporterConfig map[string]any

//counterfeiter:generate . ChangeGetter
type ChangeGetter interface {
	GetAggregateMetric() (map[string]any, error)
	Changed() bool
}

//counterfeiter:generate . ConfigWriter
type ConfigWriter interface {
	Write(ExporterConfig) error
}

//counterfeiter:generate . Runner
type Runner interface {
	IsRunning() bool
	RemovePidFile()
	Start() error
	Stop() error
}

//counterfeiter:generate . ConfigApplier
type ConfigApplier interface {
	Apply() error
}

type Manager struct {
	c ChangeGetter
	// Configuration polling interval
	d time.Duration
	w ConfigWriter
	r Runner
	a ConfigApplier
	l *logrus.Logger
}

func NewManager(c ChangeGetter, d time.Duration, w ConfigWriter, r Runner, a ConfigApplier, l *logrus.Logger) *Manager {
	return &Manager{
		c: c,
		d: d,
		w: w,
		r: r,
		a: a,
		l: l,
	}
}

func (m *Manager) Run(ctx context.Context, stopCh chan<- struct{}) {
	defer close(stopCh)

	m.l.Info("Starting OTel Manager")
	m.r.RemovePidFile()

	go func() {
		t := time.NewTicker(m.d)
		defer t.Stop()
		m.retrieveAndUpdateConfig()

		for {
			select {
			case <-t.C:
				m.retrieveAndUpdateConfig()
			}
		}
	}()

	<-ctx.Done()

	m.l.Info("Stopping OTel Manager")

	err := m.r.Stop()
	if err != nil {
		m.l.WithError(err).Error("Failed to stop Otel Collector")
	}
}

func (m *Manager) retrieveAndUpdateConfig() {
	cfg, err := m.c.GetAggregateMetric()
	if err != nil {
		m.l.WithError(err).Error("Failed to retrieve exporter configuration")
		return
	}

	running := m.r.IsRunning()

	if !m.c.Changed() && running {
		return
	}

	if err = m.w.Write(cfg); err != nil {
		m.l.WithError(err).Error("Failed to write otel collector configuration")
		return
	}

	if running {
		if err = m.a.Apply(); err != nil {
			m.l.WithError(err).Error("Failed to apply otel collector configuration")
		}
	} else {
		if err := m.r.Start(); err != nil {
			m.l.WithError(err).Error("Failed to run otel collector")
		}
	}
}
