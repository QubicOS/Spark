package kernel

import (
	"runtime"
	"sync/atomic"
)

// MaxMessageBytes is the maximum payload size for IPC messages.
const MaxMessageBytes = 1024

// Message is a fixed-size message envelope.
type Message struct {
	From Endpoint
	To   Endpoint
	Kind uint8
	Len  uint16
	Data [MaxMessageBytes]byte
}

const (
	MsgLog uint8 = iota + 1
	MsgPing
	MsgPong
	MsgNotifyShared
)

const mailboxSlots = 8

// Mailbox is a fixed-size single-producer/multi-producer, single-consumer queue.
// It is designed for bare-metal use: no allocations, busy-wait with Gosched().
type Mailbox struct {
	_     [0]func() // prevent accidental copying.
	head  atomic.Uint32
	tail  atomic.Uint32
	slots [mailboxSlots]Message
}

// TrySend attempts to enqueue a message, returning false if the mailbox is full.
func (mb *Mailbox) TrySend(msg Message) bool {
	head := mb.head.Load()
	tail := mb.tail.Load()
	if head-tail >= mailboxSlots {
		return false
	}

	// Reserve a slot.
	if !mb.head.CompareAndSwap(head, head+1) {
		return false
	}

	mb.slots[head%mailboxSlots] = msg
	return true
}

// Send enqueues a message, blocking until it succeeds.
func (mb *Mailbox) Send(msg Message) {
	for !mb.TrySend(msg) {
		runtime.Gosched()
	}
}

// TryRecv attempts to dequeue one message, returning false if empty.
func (mb *Mailbox) TryRecv() (Message, bool) {
	tail := mb.tail.Load()
	head := mb.head.Load()
	if tail == head {
		return Message{}, false
	}

	msg := mb.slots[tail%mailboxSlots]
	mb.tail.Store(tail + 1)
	return msg, true
}

// Recv blocks until one message is available.
func (mb *Mailbox) Recv() Message {
	for {
		msg, ok := mb.TryRecv()
		if ok {
			return msg
		}
		runtime.Gosched()
	}
}

