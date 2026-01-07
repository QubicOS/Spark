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
		t.advance(n)
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
	t.advance(ticks)
}

func (t *hostTime) advance(n uint64) {
	t.seq += n
	if n == 0 {
		return
	}
	// Coalesce updates: keep only the latest tick in the channel to avoid flooding.
	select {
	case t.ch <- t.seq:
		return
	default:
		select {
		case <-t.ch:
		default:
		}
		select {
		case t.ch <- t.seq:
		default:
		}
	}
}
