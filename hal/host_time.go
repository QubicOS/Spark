//go:build !tinygo

package hal

type hostTime struct {
	ch  chan uint64
	seq uint64
}

func newHostTime() *hostTime {
	return &hostTime{ch: make(chan uint64, 1024)}
}

func (t *hostTime) Ticks() <-chan uint64 { return t.ch }

func (t *hostTime) step(n uint64) {
	for i := uint64(0); i < n; i++ {
		t.seq++
		select {
		case t.ch <- t.seq:
		default:
		}
	}
}

