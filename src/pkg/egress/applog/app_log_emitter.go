package applog

import (
	"code.cloudfoundry.org/go-loggregator/v10"
)

// LogClient is used to emit logs.
type LogClient interface {
	EmitLog(message string, opts ...loggregator.EmitLogOption)
}

// AppLogEmitter abstracts the sending of a log to the application log stream.
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

// AppLogEmitterFactory is used to create new instances of AppLogEmitter
type AppLogEmitterFactory interface {
	NewAppLogEmitter(logClient LogClient, sourceIndex string) AppLogEmitter
}

// DefaultAppLogEmitterFactory implementation of AppLogEmitterFactory to produce DefaultAppLogEmitter.
type DefaultAppLogEmitterFactory struct {
}

// NewAppLogEmitter creates a new AppLogEmitter.
func (factory *DefaultAppLogEmitterFactory) NewAppLogEmitter(logClient LogClient, sourceIndex string) AppLogEmitter {
	return AppLogEmitter{
		logClient:   logClient,
		sourceIndex: sourceIndex,
	}
}

func NewAppLogEmitterFactory() DefaultAppLogEmitterFactory {
	return DefaultAppLogEmitterFactory{}
}
