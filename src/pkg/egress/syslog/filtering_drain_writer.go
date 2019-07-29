package syslog

import (
	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	"code.cloudfoundry.org/loggregator-agent/pkg/egress"
	"errors"
)

type BindingType int

const (
	BINDING_TYPE_LOG BindingType = iota
	BINDING_TYPE_METRIC
	BINDING_TYPE_ALL
	BINDING_TYPE_UNIVERSAL
)

type FilteringDrainWriter struct {
	binding Binding
	writer  egress.Writer
}

func NewFilteringDrainWriter(binding Binding, writer egress.Writer) (*FilteringDrainWriter, error) {
	if binding.Type < BINDING_TYPE_LOG || binding.Type > BINDING_TYPE_UNIVERSAL {
		return nil, errors.New("invalid binding type")
	}

	return &FilteringDrainWriter{
		binding: binding,
		writer:  writer,
	}, nil
}

func (w *FilteringDrainWriter) Write(env *loggregator_v2.Envelope) error {
	if w.binding.Type == BINDING_TYPE_UNIVERSAL {
		return w.writer.Write(env)
	}

	if env.GetTimer() != nil || env.GetEvent() != nil {
		return nil
	}

	switch w.binding.Type {
	case BINDING_TYPE_LOG:
		if env.GetLog() != nil {
			return w.writer.Write(env)
		}
	case BINDING_TYPE_METRIC:
		if env.GetCounter() != nil || env.GetGauge() != nil {
			return w.writer.Write(env)
		}
	default:
		return w.writer.Write(env)
	}

	return nil
}
