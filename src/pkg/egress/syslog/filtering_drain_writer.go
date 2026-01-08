package syslog

import (
	"errors"
	"strings"

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

// LogType defines the log types used within Cloud Foundry
// Their order in the code is as documented in https://docs.cloudfoundry.org/devguide/deploy-apps/streaming-logs.html#format
type LogType int

const (
	API LogType = iota
	STG
	RTR
	LGR
	APP
	SSH
	CELL
)

// logTypePrefixes maps string prefixes to LogType values for efficient lookup
var logTypePrefixes = map[string]LogType{
	"API":  API,
	"STG":  STG,
	"RTR":  RTR,
	"LGR":  LGR,
	"APP":  APP,
	"SSH":  SSH,
	"CELL": CELL,
}

// LogTypeSet is a set of LogTypes for efficient membership checking
type LogTypeSet map[LogType]struct{}

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
		value, ok := env.GetTags()["source_type"]
		if !ok {
			// Default to sending logs if no source_type tag is present
			value = ""
		}
		if sendsLogs(w.binding.DrainData, w.binding.LogFilter, value) {
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

// shouldIncludeLog determines if a log with the given sourceTypeTag should be forwarded
func shouldIncludeLog(logFilter *LogTypeSet, sourceTypeTag string) bool {
	// Empty filter or missing source type means no filtering
	if logFilter == nil || sourceTypeTag == "" {
		return true
	}

	// Find the first "/" to extract prefix
	idx := strings.IndexByte(sourceTypeTag, '/')
	prefix := sourceTypeTag
	if idx != -1 {
		prefix = sourceTypeTag[:idx]
	}

	// Prefer map lookup over switch for performance
	logType, known := logTypePrefixes[prefix]
	if !known {
		// Unknown log type, default to not filtering
		return true
	}

	_, exists := (*logFilter)[logType]
	return exists
}

func sendsLogs(drainData DrainData, logFilter *LogTypeSet, sourceTypeTag string) bool {
	if drainData != LOGS && drainData != LOGS_AND_METRICS && drainData != LOGS_NO_EVENTS {
		return false
	}

	if shouldIncludeLog(logFilter, sourceTypeTag) {
		return true
	}

	return false
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
