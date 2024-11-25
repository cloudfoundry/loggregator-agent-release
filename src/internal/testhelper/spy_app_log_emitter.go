package testhelper

import (
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/ingress/applog"
)

type SpyAppLogStreamFactory struct {
	logClient   applog.LogClient
	sourceIndex string
}

func (factory *SpyAppLogStreamFactory) LogClient() applog.LogClient {
	return factory.logClient
}

func (factory *SpyAppLogStreamFactory) SourceIndex() string {
	return factory.sourceIndex
}

func (factory *SpyAppLogStreamFactory) NewAppLogStream(logClient applog.LogClient, sourceIndex string) applog.AppLogStream {
	factory.logClient = logClient
	factory.sourceIndex = sourceIndex
	logStreamFactory := applog.NewAppLogStreamFactory()
	return logStreamFactory.NewAppLogStream(logClient, sourceIndex)
}
