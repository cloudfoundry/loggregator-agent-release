package syslog

import (
	"code.cloudfoundry.org/go-loggregator/v10"
)

// LogClient is used to emit logs.
type LogClient interface {
	EmitLog(message string, opts ...loggregator.EmitLogOption)
}

type LoggregatorEmitter struct {
	logClient   LogClient
	sourceIndex string
}

// WriteLog writes a message in the application log stream using a LogClient.
func (appLogEmitter *LoggregatorEmitter) WriteLog(appID string, message string) {
	if appLogEmitter.logClient == nil || appID == "" {
		return
	}

	option := loggregator.WithAppInfo(appID, "LGR", "")
	appLogEmitter.logClient.EmitLog(message, option)

	option = loggregator.WithAppInfo(
		appID,
		"SYS",
		appLogEmitter.sourceIndex,
	)
	appLogEmitter.logClient.EmitLog(message, option)
}

// NewLoggregatorEmitter creates a new LoggregatorEmitter.
func NewLoggregatorEmitter(logClient LogClient, sourceIndex string) LoggregatorEmitter {
	return LoggregatorEmitter{
		logClient:   logClient,
		sourceIndex: sourceIndex,
	}
}
