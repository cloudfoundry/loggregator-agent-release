package v2

import (
	"code.cloudfoundry.org/go-loggregator/v10"
)

// LogClient is used to emit logs - i.e. Ingress Client.
type LogClient interface {
	EmitLog(message string, opts ...loggregator.EmitLogOption)
}

// EmitAppLog writes a message in the application log stream using a LogClient.
func EmitAppLog(logClient LogClient, message string, appID string) {
	if logClient == nil || appID == "" {
		return
	}

	logclientOption := loggregator.WithAppInfo(appID, "LGR", "0")
	logClient.EmitLog(message, logclientOption)
}
