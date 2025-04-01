package testhelper

import "code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/syslog"

type SpyAppLogEmitterFactory struct {
	logClient   syslog.LogClient
	sourceIndex string
}

func (factory *SpyAppLogEmitterFactory) LogClient() syslog.LogClient {
	return factory.logClient
}

func (factory *SpyAppLogEmitterFactory) SourceIndex() string {
	return factory.sourceIndex
}

func (factory *SpyAppLogEmitterFactory) NewAppLogEmitter(logClient syslog.LogClient, sourceIndex string) syslog.AppLogEmitter {
	factory.logClient = logClient
	factory.sourceIndex = sourceIndex
	emitterFactory := syslog.NewAppLogEmitterFactory()
	return emitterFactory.NewAppLogEmitter(logClient, sourceIndex)
}
