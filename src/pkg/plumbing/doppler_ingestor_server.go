package plumbing

import "code.cloudfoundry.org/go-loggregator/v10/rpc/loggregator_v2"

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 . DopplerIngestorServerV1
type DopplerIngestorServerV1 interface {
	DopplerIngestorServer
}

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 . IngressServerV2
type IngressServerV2 interface {
	loggregator_v2.IngressServer
}
