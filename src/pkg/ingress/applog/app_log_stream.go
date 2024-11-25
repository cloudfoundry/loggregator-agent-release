package applog

import (
	"code.cloudfoundry.org/go-loggregator/v10"
)

// LogClient is used to emit logs - i.e. Ingress Client.
type LogClient interface {
	EmitLog(message string, opts ...loggregator.EmitLogOption)
}

// AppLogStream abstracts the sending of a log to the application log stream.
type AppLogStream struct {
	logClient   LogClient
	sourceIndex string
}

// Emit writes a message in the application log stream using a LogClient.
func (appLogStream *AppLogStream) Emit(message string, appID string) {
	if appLogStream.logClient == nil || appID == "" {
		return
	}

	logclientOption := loggregator.WithAppInfo(appID, "LGR", "")
	appLogStream.logClient.EmitLog(message, logclientOption)

	logclientOption = loggregator.WithAppInfo(
		appID,
		"SYS",
		appLogStream.sourceIndex,
	)
	appLogStream.logClient.EmitLog(message, logclientOption)
}

// AppLogStreamFactory is used to create new instances of AppLogStream
type AppLogStreamFactory interface {
	NewAppLogStream(logClient LogClient, sourceIndex string) AppLogStream
}

// DefaultLogStreamFactory implementation of AppLogStreamFactory.
type DefaultLogStreamFactory struct {
}

// NewAppLogStream creates a new AppLogStream.
func (factory *DefaultLogStreamFactory) NewAppLogStream(logClient LogClient, sourceIndex string) AppLogStream {
	return AppLogStream{
		logClient:   logClient,
		sourceIndex: sourceIndex,
	}
}

func NewAppLogStreamFactory() DefaultLogStreamFactory {
	return DefaultLogStreamFactory{}
}
