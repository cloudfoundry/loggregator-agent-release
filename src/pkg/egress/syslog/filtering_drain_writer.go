package syslog

import (
	"errors"

	"code.cloudfoundry.org/go-loggregator/v10/rpc/loggregator_v2"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress"
)

type DrainData int

const (
	LOGS DrainData = iota
	METRICS
	TRACES
	ALL
	LOGS_NO_EVENTS
	LOGS_AND_METRICS
)

type FilteringDrainWriter struct {
	binding Binding
	writer  egress.Writer
}

func NewFilteringDrainWriter(binding Binding, writer egress.Writer) (*FilteringDrainWriter, error) {
	if binding.DrainData < LOGS || binding.DrainData > LOGS_AND_METRICS {
		return nil, errors.New("invalid binding type")
	}

	return &FilteringDrainWriter{
		binding: binding,
		writer:  writer,
	}, nil
}

func (w *FilteringDrainWriter) Write(env *loggregator_v2.Envelope) error {
	if w.binding.DrainData == ALL {
		return w.writer.Write(env)
	}

	if env.GetTimer() != nil {
		if w.binding.DrainData == TRACES {
			return w.writer.Write(env)
		}
	}
	if env.GetEvent() != nil {
		if w.binding.DrainData == LOGS {
			return w.writer.Write(env)
		}
	}
	if env.GetLog() != nil {
		if sendsLogs(w.binding.DrainData) {
			return w.writer.Write(env)
		}
	}
	if env.GetCounter() != nil || env.GetGauge() != nil {
		if sendsMetrics(w.binding.DrainData) {
			return w.writer.Write(env)
		}
	}

	return nil
}

func sendsLogs(drainData DrainData) bool {
	switch drainData {
	case LOGS:
		return true
	case LOGS_AND_METRICS:
		return true
	case LOGS_NO_EVENTS:
		return true
	default:
		return false
	}
}

func sendsMetrics(drainData DrainData) bool {
	switch drainData {
	case LOGS_AND_METRICS:
		return true
	case METRICS:
		return true
	default:
		return false
	}
}
