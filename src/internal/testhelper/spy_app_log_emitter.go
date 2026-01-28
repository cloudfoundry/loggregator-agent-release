package testhelper

import (
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/loggregator"
)

type SpyAppLogEmitterFactory struct {
	logClient   loggregator.LogClient
	sourceIndex string
}

func (factory *SpyAppLogEmitterFactory) LogClient() loggregator.LogClient {
	return factory.logClient
}

func (factory *SpyAppLogEmitterFactory) SourceIndex() string {
	return factory.sourceIndex
}

func (factory *SpyAppLogEmitterFactory) NewLogEmitter(logClient loggregator.LogClient, sourceIndex string) loggregator.LogStream {
	factory.logClient = logClient
	factory.sourceIndex = sourceIndex
	emitterFactory := loggregator.NewAppLogStreamFactory()
	return emitterFactory.NewLogEmitter(logClient, sourceIndex)
}
