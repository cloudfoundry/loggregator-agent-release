package egress

import (
	"io"

	"golang.org/x/net/context"

	gendiodes "code.cloudfoundry.org/go-diodes"
	"code.cloudfoundry.org/go-loggregator/v9/rpc/loggregator_v2"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/diodes"
)

type WaitGroup interface {
	Add(delta int)
	Done()
}

type Writer interface {
	Write(*loggregator_v2.Envelope) error
}

type WriteCloser interface {
	Write(*loggregator_v2.Envelope) error
	io.Closer
}

type DiodeWriter struct {
	wc    WriteCloser
	diode *diodes.OneToOneEnvelopeV2
	wg    WaitGroup

	ctx context.Context
}

func NewDiodeWriter(
	ctx context.Context,
	wc WriteCloser,
	alerter gendiodes.Alerter,
	wg WaitGroup,
) *DiodeWriter {
	dw := &DiodeWriter{
		wc:    wc,
		diode: diodes.NewOneToOneEnvelopeV2(10000, alerter, gendiodes.WithWaiterContext(ctx)),
		wg:    wg,
		ctx:   ctx,
	}
	wg.Add(1)
	go dw.start()

	return dw
}

// Write writes an envelope into the diode. This can not fail.
func (d *DiodeWriter) Write(env *loggregator_v2.Envelope) error {
	d.diode.Set(env)

	return nil
}

func (d *DiodeWriter) start() {
	defer d.wc.Close()
	defer d.wg.Done()

	for {
		e := d.diode.Next()
		if e == nil {
			return
		}

		err := d.wc.Write(e)
		if err != nil && ContextDone(d.ctx) {
			return
		}
	}
}

func ContextDone(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}
