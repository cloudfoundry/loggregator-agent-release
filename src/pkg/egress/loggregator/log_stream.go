package loggregator

import (
	"code.cloudfoundry.org/go-loggregator/v10"
)

// LogClient is used to emit logs.
type LogClient interface {
	EmitLog(message string, opts ...loggregator.EmitLogOption)
}

type LogType interface {
	appID() string
}

type appLog struct {
	appIdentifier string
}

func (a *appLog) appID() string {
	return a.appIdentifier
}

type platformLog struct {
}

func (p *platformLog) appID() string {
	return ""
}

func ForApp(appIdentifier string) LogType {
	return &appLog{appIdentifier: appIdentifier}
}

func ForPlatform() LogType {
	return &platformLog{}
}

// LogStream abstracts the sending of a log to the application log stream.
type LogStream struct {
	logClient   LogClient
	sourceIndex string
}

// Emit writes a message in the application log stream using a LogClient.
func (logStream *LogStream) Emit(message string, option LogType) {
	if logStream.logClient == nil {
		return
	}

	logclientOption := loggregator.WithAppInfo(option.appID(), "LGR", "")
	logStream.logClient.EmitLog(message, logclientOption)

	logclientOption = loggregator.WithAppInfo(
		option.appID(),
		"SYS",
		logStream.sourceIndex,
	)
	logStream.logClient.EmitLog(message, logclientOption)
}

// LogStreamFactory is used to create new instances of LogStream
type LogStreamFactory interface {
	NewLogEmitter(logClient LogClient, sourceIndex string) LogStream
}

// DefaultLogEmitterFactory implementation of LogStreamFactory to produce DefaultAppLogEmitter.
type DefaultLogEmitterFactory struct {
}

// NewAppLogEmitter creates a new LogStream.
func (factory *DefaultLogEmitterFactory) NewLogEmitter(logClient LogClient, sourceIndex string) LogStream {
	return LogStream{
		logClient:   logClient,
		sourceIndex: sourceIndex,
	}
}

func NewAppLogStreamFactory() DefaultLogEmitterFactory {
	return DefaultLogEmitterFactory{}
}
