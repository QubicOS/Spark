package kernel

// Context provides task-local access to kernel operations.
type Context struct {
	k      *Kernel
	taskID TaskID
}

// TaskID returns the current task ID.
func (c *Context) TaskID() TaskID { return c.taskID }

// RecvChan returns the inbound message channel for an endpoint capability.
func (c *Context) RecvChan(epCap Capability) (<-chan Message, bool) {
	if !epCap.valid() || !epCap.canRecv() {
		return nil, false
	}

	c.k.mu.Lock()
	if epCap.ep >= c.k.endpointCount {
		c.k.mu.Unlock()
		return nil, false
	}
	ch := c.k.endpoints[epCap.ep].ch
	c.k.mu.Unlock()
	if ch == nil {
		return nil, false
	}
	return ch, true
}

// Recv reads one message from the capability endpoint, blocking until a message arrives.
func (c *Context) Recv(epCap Capability) (Message, bool) {
	ch, ok := c.RecvChan(epCap)
	if !ok {
		return Message{}, false
	}
	return <-ch, true
}

// TryRecv reads one message from the capability endpoint without blocking.
func (c *Context) TryRecv(epCap Capability) (Message, bool) {
	ch, ok := c.RecvChan(epCap)
	if !ok {
		return Message{}, false
	}
	select {
	case msg := <-ch:
		return msg, true
	default:
		return Message{}, false
	}
}

// BlockOnTick blocks the task until the next Kernel.Tick call.
func (c *Context) BlockOnTick() {
	if c.k == nil {
		return
	}
	after := c.k.nowTick()
	_ = c.k.waitTick(after)
}

// Send sends a message to the capability endpoint.
func (c *Context) Send(fromCap, toCap Capability, kind uint16, payload []byte) bool {
	return c.SendCap(fromCap, toCap, kind, payload, Capability{})
}

// SendCap sends a message and transfers an optional capability.
func (c *Context) SendCap(fromCap, toCap Capability, kind uint16, payload []byte, xfer Capability) bool {
	return c.SendCapResult(fromCap, toCap, kind, payload, xfer) == SendOK
}

// SendCapResult sends a message and transfers an optional capability.
func (c *Context) SendCapResult(fromCap, toCap Capability, kind uint16, payload []byte, xfer Capability) SendResult {
	if !fromCap.valid() {
		return SendErrInvalidFromCap
	}
	if !fromCap.canSend() {
		return SendErrFromNoSendRight
	}
	if !toCap.valid() {
		return SendErrInvalidToCap
	}
	if !toCap.canSend() {
		return SendErrToNoSendRight
	}
	return c.k.send(fromCap.ep, toCap.ep, kind, payload, xfer)
}

// SendTo sends a message to the capability endpoint.
//
// The message From field is set to 0 (unknown).
func (c *Context) SendTo(toCap Capability, kind uint16, payload []byte) bool {
	return c.SendToCap(toCap, kind, payload, Capability{})
}

// SendToCap sends a message and transfers an optional capability.
//
// The message From field is set to 0 (unknown).
func (c *Context) SendToCap(toCap Capability, kind uint16, payload []byte, xfer Capability) bool {
	return c.SendToCapResult(toCap, kind, payload, xfer) == SendOK
}

// SendToCapResult sends a message and transfers an optional capability.
//
// The message From field is set to 0 (unknown).
func (c *Context) SendToCapResult(toCap Capability, kind uint16, payload []byte, xfer Capability) SendResult {
	if !toCap.valid() {
		return SendErrInvalidToCap
	}
	if !toCap.canSend() {
		return SendErrToNoSendRight
	}
	return c.k.send(0, toCap.ep, kind, payload, xfer)
}

// NewEndpoint allocates a new endpoint and returns a capability for it.
func (c *Context) NewEndpoint(rights Rights) Capability {
	if c.k == nil {
		return Capability{}
	}
	return c.k.NewEndpoint(rights)
}

// NowTick returns the last observed tick value.
func (c *Context) NowTick() uint64 {
	if c.k == nil {
		return 0
	}
	return c.k.nowTick()
}

// WaitTick blocks until tick advances past the provided value and returns the new tick.
func (c *Context) WaitTick(after uint64) uint64 {
	if c.k == nil {
		return 0
	}
	return c.k.waitTick(after)
}
