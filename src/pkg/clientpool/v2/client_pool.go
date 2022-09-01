package v2

import (
	"crypto/rand"
	"errors"
	"math"
	"math/big"
	"sync/atomic"
	"unsafe"

	"code.cloudfoundry.org/go-loggregator/v9/rpc/loggregator_v2"
)

type Conn interface {
	Write(data []*loggregator_v2.Envelope) (err error)
}

type ClientPool struct {
	conns []unsafe.Pointer
}

func New(conns ...Conn) *ClientPool {
	pool := &ClientPool{
		conns: make([]unsafe.Pointer, len(conns)),
	}
	for i := range conns {
		pool.conns[i] = unsafe.Pointer(&conns[i])
	}

	return pool
}

func (c *ClientPool) Write(msgs []*loggregator_v2.Envelope) error {
	nBig, err := rand.Int(rand.Reader, big.NewInt(math.MaxInt32))
	if err != nil {
		return err
	}
	seed := int(nBig.Int64())
	for i := range c.conns {
		idx := (i + seed) % len(c.conns)
		conn := *(*Conn)(atomic.LoadPointer(&c.conns[idx]))

		if err := conn.Write(msgs); err == nil {
			return nil
		}
	}

	return errors.New("unable to write to any dopplers")
}
