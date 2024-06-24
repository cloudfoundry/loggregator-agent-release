package egress_test

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/go-loggregator/v10/rpc/loggregator_v2"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress"
)

var _ = Describe("DiodeWriter", func() {
	It("dispatches calls to write to the underlying writer", func() {
		spyWaitGroup := &SpyWaitGroup{}
		expectedEnv := &loggregator_v2.Envelope{
			SourceId: "test-source-id",
		}
		spyWriter := &SpyWriter{}
		spyAlerter := &SpyAlerter{}
		dw := egress.NewDiodeWriter(context.TODO(), spyWriter, spyAlerter, spyWaitGroup)

		_ = dw.Write(expectedEnv)

		Eventually(spyWriter.calledWith).Should(Equal([]*loggregator_v2.Envelope{
			expectedEnv,
		}))
	})

	It("dispatches calls to close to the underlying writer", func() {
		spyWaitGroup := &SpyWaitGroup{}
		spyWriter := &SpyWriter{}
		spyAlerter := &SpyAlerter{}
		ctx, cancel := context.WithCancel(context.TODO())

		egress.NewDiodeWriter(ctx, spyWriter, spyAlerter, spyWaitGroup)

		cancel()

		Eventually(spyWriter.CloseCalled).Should(Equal(int64(1)))
	})

	It("is not blocked when underlying writer is blocked", func() {
		done := make(chan interface{})
		go func() {
			defer GinkgoRecover()
			defer close(done)
			spyWaitGroup := &SpyWaitGroup{}
			spyWriter := &SpyWriter{
				blockWrites: true,
			}
			spyAlerter := &SpyAlerter{}
			dw := egress.NewDiodeWriter(context.TODO(), spyWriter, spyAlerter, spyWaitGroup)
			_ = dw.Write(nil)
		}()
		Eventually(done).Should(BeClosed())
	})

	It("flushes existing messages after close", func() {
		spyWaitGroup := &SpyWaitGroup{}
		spyWriter := &SpyWriter{
			blockWrites: true,
		}
		spyAlerter := &SpyAlerter{}
		ctx, cancel := context.WithCancel(context.TODO())

		dw := egress.NewDiodeWriter(ctx, spyWriter, spyAlerter, spyWaitGroup)

		e := &loggregator_v2.Envelope{}
		for i := 0; i < 100; i++ {
			_ = dw.Write(e)
		}
		cancel()
		spyWriter.WriteBlocked(false)

		Eventually(spyWriter.calledWith).Should(HaveLen(100))
	})

	It("closes the writer if write returns an error and context is done", func() {
		spyWaitGroup := &SpyWaitGroup{}
		spyWriter := &SpyWriter{
			writeError: fmt.Errorf("some-error"),
		}
		spyAlerter := &SpyAlerter{}
		ctx, cancel := context.WithCancel(context.TODO())
		dw := egress.NewDiodeWriter(ctx, spyWriter, spyAlerter, spyWaitGroup)

		go func() {
			for {
				_ = dw.Write(&loggregator_v2.Envelope{})
			}
		}()

		Eventually(spyWriter.calledWith).ShouldNot(BeEmpty())
		cancel()

		Eventually(spyWriter.CloseCalled).ShouldNot(BeZero())
	})

	It("registers with the wait group and deregisters when done", func() {
		spyWaitGroup := &SpyWaitGroup{}
		spyWriter := &SpyWriter{
			blockWrites: true,
		}
		spyAlerter := &SpyAlerter{}
		ctx, cancel := context.WithCancel(context.TODO())

		egress.NewDiodeWriter(ctx, spyWriter, spyAlerter, spyWaitGroup)

		Eventually(spyWaitGroup.AddInput).Should(Equal(int64(1)))
		Expect(spyWaitGroup.DoneCalled()).To(Equal(int64(0)))
		cancel()
		Eventually(spyWaitGroup.DoneCalled).Should(Equal(int64(1)))
	})
})

type SpyWriter struct {
	mu          sync.Mutex
	calledWith_ []*loggregator_v2.Envelope
	closeCalled int64
	closeRet    error
	blockWrites bool
	writeError  error
}

func (s *SpyWriter) Write(env *loggregator_v2.Envelope) error {
	for {
		s.mu.Lock()
		block := s.blockWrites
		s.mu.Unlock()

		if block {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		break
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.calledWith_ = append(s.calledWith_, env)
	return s.writeError
}

func (s *SpyWriter) WriteBlocked(blocked bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.blockWrites = blocked
}

func (s *SpyWriter) Close() error {
	atomic.AddInt64(&s.closeCalled, 1)

	return s.closeRet
}

func (s *SpyWriter) CloseCalled() int64 {
	return atomic.LoadInt64(&s.closeCalled)
}

func (s *SpyWriter) calledWith() []*loggregator_v2.Envelope {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]*loggregator_v2.Envelope, len(s.calledWith_))
	copy(result, s.calledWith_)
	return result
}

type SpyAlerter struct {
	missed_ int64
}

func (s *SpyAlerter) Alert(missed int) {
	atomic.AddInt64(&s.missed_, int64(missed))
}

type SpyWaitGroup struct {
	addInput   int64
	doneCalled int64
}

func (s *SpyWaitGroup) Add(delta int) {
	atomic.AddInt64(&s.addInput, int64(delta))
}

func (s *SpyWaitGroup) Done() {
	atomic.AddInt64(&s.doneCalled, 1)
}

func (s *SpyWaitGroup) AddInput() int64 {
	return atomic.LoadInt64(&s.addInput)
}

func (s *SpyWaitGroup) DoneCalled() int64 {
	return atomic.LoadInt64(&s.doneCalled)
}
