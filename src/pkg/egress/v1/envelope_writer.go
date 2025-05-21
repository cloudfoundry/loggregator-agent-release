package v1

import "github.com/cloudfoundry/sonde-go/events"

//go:generate go tool counterfeiter -generate
//counterfeiter:generate . EnvelopeWriter
type EnvelopeWriter interface {
	Write(event *events.Envelope)
}
