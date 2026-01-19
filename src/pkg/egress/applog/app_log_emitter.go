package applog

import (
	"code.cloudfoundry.org/go-loggregator/v10"
)

// LogClient is used to emit logs.
type LogClient interface {
	EmitLog(message string, opts ...loggregator.EmitLogOption)
}

// LogEmitter abstracts the sending of a log to the application log stream.
type LogEmitter struct {
	logClient   LogClient
	sourceIndex string
}

// EmitAppLog writes a message in the application log stream using a LogClient.
func (logEmitter *LogEmitter) EmitAppLog(appID string, message string) {
	if logEmitter.logClient == nil || appID == "" {
		return
	}

	option := loggregator.WithAppInfo(appID, "LGR", "")
	logEmitter.logClient.EmitLog(message, option)

	option = loggregator.WithAppInfo(
		appID,
		"SYS",
		logEmitter.sourceIndex,
	)
	logEmitter.logClient.EmitLog(message, option)
}

// LogEmitterFactory is used to create new instances of LogEmitter
type LogEmitterFactory interface {
	NewLogEmitter(logClient LogClient, sourceIndex string) LogEmitter
}

// DefaultLogEmitterFactory implementation of LogEmitterFactory to produce DefaultAppLogEmitter.
type DefaultLogEmitterFactory struct {
}

// NewAppLogEmitter creates a new LogEmitter.
func (factory *DefaultLogEmitterFactory) NewLogEmitter(logClient LogClient, sourceIndex string) LogEmitter {
	return LogEmitter{
		logClient:   logClient,
		sourceIndex: sourceIndex,
	}
}

func NewAppLogEmitterFactory() DefaultLogEmitterFactory {
	return DefaultLogEmitterFactory{}
}
