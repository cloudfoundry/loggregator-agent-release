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

func (factory *SpyAppLogEmitterFactory) NewAppLogEmitter(logClient applog.LogClient, sourceIndex string) applog.AppLogEmitter {
	factory.logClient = logClient
	factory.sourceIndex = sourceIndex
	emitterFactory := applog.NewAppLogEmitterFactory()
	return emitterFactory.NewAppLogEmitter(logClient, sourceIndex)
}
