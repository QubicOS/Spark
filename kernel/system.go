package kernel

import (
	"runtime"
	"sync/atomic"
	"time"
)

// System is the v0 microkernel state: endpoints, mailboxes, and timebase.
type System struct {
	mbox   [4]Mailbox
	shared SharedBuffer
	ticks  atomic.Uint64
}

// NewSystem creates a kernel instance.
func NewSystem() *System {
	return &System{}
}

// StartTick starts a 1ms ticker that increments the kernel tick counter.
// This is a measurement/timebase primitive for v0.
func (s *System) StartTick() {
	go func() {
		t := time.NewTicker(1 * time.Millisecond)
		defer t.Stop()
		for range t.C {
			s.ticks.Add(1)
		}
	}()
}

// Ticks returns the current tick count (1ms per tick).
func (s *System) Ticks() uint64 {
	return s.ticks.Load()
}

// Shared returns the shared buffer (hybrid IPC).
func (s *System) Shared() *SharedBuffer {
	return &s.shared
}

// Send copies the payload into a fixed-size message and enqueues it.
func (s *System) Send(from, to Endpoint, kind uint8, payload []byte) {
	var msg Message
	msg.From = from
	msg.To = to
	msg.Kind = kind
	if len(payload) > 0 {
		if len(payload) > MaxMessageBytes {
			payload = payload[:MaxMessageBytes]
		}
		msg.Len = uint16(len(payload))
		copy(msg.Data[:], payload)
	}
	s.mbox[to].Send(msg)
}

// Recv blocks until a message is available for the endpoint.
func (s *System) Recv(to Endpoint) Message {
	return s.mbox[to].Recv()
}

// Yield yields execution to let other tasks run.
func (s *System) Yield() {
	runtime.Gosched()
}
