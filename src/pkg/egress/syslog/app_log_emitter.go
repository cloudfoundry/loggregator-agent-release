package syslog

import (
	"code.cloudfoundry.org/go-loggregator/v10"
)

// LogClient is used to emit logs.
type LogClient interface {
	EmitLog(message string, opts ...loggregator.EmitLogOption)
}

type AppLogEmitter struct {
	logClient   LogClient
	sourceIndex string
}

// EmitLog writes a message in the application log stream using a LogClient.
func (appLogEmitter *AppLogEmitter) EmitLog(appID string, message string) {
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

// NewAppLogEmitter creates a new AppLogEmitter.
func NewAppLogEmitter(logClient LogClient, sourceIndex string) AppLogEmitter {
	return AppLogEmitter{
		logClient:   logClient,
		sourceIndex: sourceIndex,
	}
}
