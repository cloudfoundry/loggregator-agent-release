package v1

import "github.com/cloudfoundry/sonde-go/events"

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 . EnvelopeWriter
type EnvelopeWriter interface {
	Write(event *events.Envelope)
}
