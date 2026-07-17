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
	if env.GetTimer() != nil {
		if w.binding.DrainData == TRACES || w.binding.DrainData == ALL {
			return w.writer.Write(env)
		}
	}
	if env.GetEvent() != nil {
		if sendsEvents(w.binding.DrainData, w.binding.LogFilter, env.GetTags()["source_type"]) {
			return w.writer.Write(env)
		}
	}
	if env.GetLog() != nil {
		if sendsLogs(w.binding.DrainData, w.binding.LogFilter, env.GetTags()["source_type"]) {
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

func sendsEvents(drainData DrainData, logFilter *LogFilter, sourceTypeTag string) bool {
	if drainData != LOGS && drainData != ALL {
		return false
	}

	return logFilter.ShouldInclude(sourceTypeTag)
}

func sendsLogs(drainData DrainData, logFilter *LogFilter, sourceTypeTag string) bool {
	if drainData == TRACES || drainData == METRICS {
		return false
	}

	return logFilter.ShouldInclude(sourceTypeTag)
}

func sendsMetrics(drainData DrainData) bool {
	if drainData != LOGS_AND_METRICS && drainData != METRICS && drainData != ALL {
		return false
	}
	return true
}
