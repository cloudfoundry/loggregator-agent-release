package testhelper

import (
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/applog"
)

type SpyAppLogEmitterFactory struct {
	logClient   applog.LogClient
	sourceIndex string
}

func (factory *SpyAppLogEmitterFactory) LogClient() applog.LogClient {
	return factory.logClient
}

func (factory *SpyAppLogEmitterFactory) SourceIndex() string {
	return factory.sourceIndex
}

func (factory *SpyAppLogEmitterFactory) NewLogEmitter(logClient applog.LogClient, sourceIndex string) applog.LogEmitter {
	factory.logClient = logClient
	factory.sourceIndex = sourceIndex
	emitterFactory := applog.NewAppLogEmitterFactory()
	return emitterFactory.NewLogEmitter(logClient, sourceIndex)
}
