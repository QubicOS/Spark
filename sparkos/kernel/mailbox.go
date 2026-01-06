package kernel

type mailbox struct {
	head  uint8
	tail  uint8
	slots [mailboxSlots]Message
}

func (mb *mailbox) push(msg Message) bool {
	if mb.head-mb.tail >= mailboxSlots {
		return false
	}
	mb.slots[mb.head%mailboxSlots] = msg
	mb.head++
	return true
}

func (mb *mailbox) pop() (Message, bool) {
	if mb.tail == mb.head {
		return Message{}, false
	}
	msg := mb.slots[mb.tail%mailboxSlots]
	mb.tail++
	return msg, true
}
