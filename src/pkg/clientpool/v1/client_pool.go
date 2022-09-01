package v1

import (
	"crypto/rand"
	"errors"
	"math"
	"math/big"
	"sync/atomic"
	"unsafe"
)

type Conn interface {
	Write(data []byte) (err error)
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

func (c *ClientPool) Write(msg []byte) error {
	nBig, err := rand.Int(rand.Reader, big.NewInt(math.MaxInt32))
	if err != nil {
		return err
	}
	seed := int(nBig.Int64())
	for i := range c.conns {
		idx := (i + seed) % len(c.conns)
		conn := *(*Conn)(atomic.LoadPointer(&c.conns[idx]))

		if err := conn.Write(msg); err == nil {
			return nil
		}
	}

	return errors.New("unable to write to any dopplers")
}
