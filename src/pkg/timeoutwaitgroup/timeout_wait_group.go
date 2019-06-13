package timeoutwaitgroup

import (
	"sync"
	"time"
)

// TimeoutWaitGroup is a wrapper around sync.WaitGroup that will wait for no
// longer than a given duration.
type TimeoutWaitGroup struct {
	wg      sync.WaitGroup
	timeout time.Duration
}

// New returns a new TimeoutWaitGroup with the given timeout period.
func New(timeout time.Duration) *TimeoutWaitGroup {
	return &TimeoutWaitGroup{
		timeout: timeout,
	}
}

// Wait will return after the configured timeout or once everything in the
// group is done, whichever comes first.
func (s *TimeoutWaitGroup) Wait() {
	done := make(chan struct{})

	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(s.timeout):
	}
}

// Add adds items to the WaitGroup.
func (s *TimeoutWaitGroup) Add(delta int) {
	s.wg.Add(delta)
}

// Done removes a single item from the WaitGroup.
func (s *TimeoutWaitGroup) Done() {
	s.wg.Done()
}
