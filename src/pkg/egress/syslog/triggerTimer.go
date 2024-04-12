package syslog

import "time"

type TriggerTimer struct {
	trigger chan int
	running bool
}

type Timer interface {
	Start(d time.Duration, f func())
}

func NewTriggerTimer() Timer {
	return &TriggerTimer{
		running: false,
	}
}

func (t *TriggerTimer) Start(d time.Duration, f func()) {
	t.running = true
	for {
		timer := time.NewTimer(d)
		select {
		case <-timer.C:
		case <-t.trigger:
			f()
			t.running = false
		}
	}
}

func (t *TriggerTimer) Trigger() {
	t.trigger <- 1
}

func (t *TriggerTimer) Running() bool {
	return t.running
}
