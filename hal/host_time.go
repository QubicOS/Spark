//go:build !tinygo

package hal

import "time"

type hostTime struct {
	ch  chan uint64
	seq uint64

	last time.Time
	acc  time.Duration
}

func newHostTime() *hostTime {
	return &hostTime{ch: make(chan uint64, 1024)}
}

func (t *hostTime) Ticks() <-chan uint64 { return t.ch }

func (t *hostTime) step(n uint64) {
	now := time.Now()
	if t.last.IsZero() {
		t.last = now
		t.acc = 0
		t.stepN(n)
		return
	}

	t.acc += now.Sub(t.last)
	t.last = now

	const tickDur = time.Millisecond
	ticks := uint64(t.acc / tickDur)
	if ticks == 0 {
		return
	}
	t.acc = t.acc % tickDur
	t.stepN(ticks)
}

func (t *hostTime) stepN(n uint64) {
	for i := uint64(0); i < n; i++ {
		t.seq++
		select {
		case t.ch <- t.seq:
		default:
		}
	}
}
