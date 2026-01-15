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

type mailboxSlot struct {
	seq atomic.Uint32
	msg Message
}

// Mailbox is a fixed-size multi-producer, single-consumer queue.
// It is designed for bare-metal use: no allocations, busy-wait with Gosched().
//
// The zero value is ready to use.
type Mailbox struct {
	_     [0]func() // prevent accidental copying.
	head  atomic.Uint32
	tail  atomic.Uint32
	init  atomic.Uint32
	slots [mailboxSlots]mailboxSlot
}

func (mb *Mailbox) ensureInit() {
	if mb.init.Load() == 2 {
		return
	}

	if mb.init.CompareAndSwap(0, 1) {
		for i := range mb.slots {
			mb.slots[i].seq.Store(uint32(i))
		}
		mb.init.Store(2)
		return
	}

	for mb.init.Load() != 2 {
		runtime.Gosched()
	}
}

// TrySend attempts to enqueue a message, returning false if the mailbox is full.
func (mb *Mailbox) TrySend(msg Message) bool {
	mb.ensureInit()

	for {
		head := mb.head.Load()
		slot := &mb.slots[head%mailboxSlots]
		seq := slot.seq.Load()
		diff := int32(seq) - int32(head)

		switch {
		case diff == 0:
			if !mb.head.CompareAndSwap(head, head+1) {
				continue
			}
			slot.msg = msg
			slot.seq.Store(head + 1)
			return true
		case diff < 0:
			return false
		default:
			continue
		}
	}
}

// Send enqueues a message, blocking until it succeeds.
func (mb *Mailbox) Send(msg Message) {
	for !mb.TrySend(msg) {
		runtime.Gosched()
	}
}

// TryRecv attempts to dequeue one message, returning false if empty.
func (mb *Mailbox) TryRecv() (Message, bool) {
	mb.ensureInit()

	for {
		tail := mb.tail.Load()
		slot := &mb.slots[tail%mailboxSlots]
		seq := slot.seq.Load()
		diff := int32(seq) - int32(tail+1)

		switch {
		case diff == 0:
			if !mb.tail.CompareAndSwap(tail, tail+1) {
				continue
			}
			msg := slot.msg
			slot.seq.Store(tail + mailboxSlots)
			return msg, true
		case diff < 0:
			return Message{}, false
		default:
			continue
		}
	}
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
