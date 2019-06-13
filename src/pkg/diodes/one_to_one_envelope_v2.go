package diodes

import (
	gendiodes "code.cloudfoundry.org/go-diodes"
	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
)

// OneToOneEnvelopeV2 diode is optimized for a single writer and a single
// reader for byte slices.
type OneToOneEnvelopeV2 struct {
	d *gendiodes.Waiter
}

// NewOneToOneEnvelopeV2 initializes a new one to one diode of a given size
// and alerter.  The alerter is called whenever data is dropped with an
// integer representing the number of byte slices that were dropped.
func NewOneToOneEnvelopeV2(size int, alerter gendiodes.Alerter, opts ...gendiodes.WaiterConfigOption) *OneToOneEnvelopeV2 {
	return &OneToOneEnvelopeV2{
		d: gendiodes.NewWaiter(gendiodes.NewOneToOne(size, alerter), opts...),
	}
}

// Set inserts the given data into the diode.
func (d *OneToOneEnvelopeV2) Set(data *loggregator_v2.Envelope) {
	d.d.Set(gendiodes.GenericDataType(data))
}

// TryNext returns the next item to be read from the diode. If the diode is
// empty it will return a nil slice of bytes and false for the bool.
func (d *OneToOneEnvelopeV2) TryNext() (*loggregator_v2.Envelope, bool) {
	data, ok := d.d.TryNext()
	if !ok {
		return nil, ok
	}

	return (*loggregator_v2.Envelope)(data), true
}

// Next will return the next item to be read from the diode. If the diode is
// empty this method will block until an item is available to be read.
func (d *OneToOneEnvelopeV2) Next() *loggregator_v2.Envelope {
	data := d.d.Next()
	return (*loggregator_v2.Envelope)(data)
}
