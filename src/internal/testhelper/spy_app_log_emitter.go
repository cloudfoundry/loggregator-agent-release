package testhelper

import (
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/loggregator"
)

type SpyLogStreamFactory struct {
	logClient   loggregator.LogClient
	sourceIndex string
}

func (factory *SpyLogStreamFactory) LogClient() loggregator.LogClient {
	return factory.logClient
}

func (factory *SpyLogStreamFactory) SourceIndex() string {
	return factory.sourceIndex
}

func (factory *SpyLogStreamFactory) NewLogStream(logClient loggregator.LogClient, sourceIndex string) loggregator.LogStream {
	factory.logClient = logClient
	factory.sourceIndex = sourceIndex
	logStreamFactory := loggregator.NewAppLogStreamFactory()
	return logStreamFactory.NewLogStream(logClient, sourceIndex)
}
