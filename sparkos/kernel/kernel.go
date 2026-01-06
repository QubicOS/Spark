package kernel

import "sync"

const (
	maxTasks     = 32
	maxEndpoints = 32
	mailboxSlots = 8
)

type TaskID uint8

// Rights define which operations are allowed for a capability.
type Rights uint8

const (
	RightSend Rights = 1 << iota
	RightRecv
)

// Endpoint identifies an IPC destination.
type Endpoint uint8

// Capability grants access to an IPC endpoint.
//
// It is opaque by construction (no exported fields) and may be transferred via IPC.
type Capability struct {
	ep     Endpoint
	rights Rights
}

func (c Capability) valid() bool {
	return c.rights != 0
}

func (c Capability) Valid() bool { return c.valid() }

func (c Capability) canSend() bool { return c.rights&RightSend != 0 }
func (c Capability) canRecv() bool { return c.rights&RightRecv != 0 }

// Restrict returns a capability with a reduced set of rights.
func (c Capability) Restrict(rights Rights) Capability {
	if !c.valid() {
		return Capability{}
	}
	r := c.rights & rights
	if r == 0 {
		return Capability{}
	}
	return Capability{ep: c.ep, rights: r}
}

// Message is a fixed-size IPC envelope.
type Message struct {
	From Endpoint
	To   Endpoint
	Kind uint16
	Len  uint16
	Data [MaxMessageBytes]byte
	Cap  Capability
}

// MaxMessageBytes is the maximum payload size for IPC messages.
//
// Larger transfers should use shared buffers + notify protocols, not mailbox copies.
const MaxMessageBytes = 128

// SendResult describes the outcome of a send attempt.
type SendResult uint8

const (
	SendOK SendResult = iota
	SendErrInvalidFromCap
	SendErrInvalidToCap
	SendErrFromNoSendRight
	SendErrToNoSendRight
	SendErrNoEndpoint
	SendErrPayloadTooLarge
	SendErrQueueFull
)

func (r SendResult) String() string {
	switch r {
	case SendOK:
		return "ok"
	case SendErrInvalidFromCap:
		return "invalid from capability"
	case SendErrInvalidToCap:
		return "invalid to capability"
	case SendErrFromNoSendRight:
		return "from capability has no send right"
	case SendErrToNoSendRight:
		return "to capability has no send right"
	case SendErrNoEndpoint:
		return "no such endpoint"
	case SendErrPayloadTooLarge:
		return "payload too large"
	case SendErrQueueFull:
		return "queue full"
	default:
		return "unknown"
	}
}

// Task is a unit of execution.
type Task interface {
	Run(*Context)
}

type endpointState struct {
	ch chan Message
}

type taskState struct {
	task Task
}

// Kernel is a minimal IPC router plus endpoint allocator.
type Kernel struct {
	mu sync.Mutex

	endpoints     [maxEndpoints]endpointState
	endpointCount Endpoint

	tasks     [maxTasks]taskState
	taskCount TaskID

	tick     uint64
	tickCond *sync.Cond
}

// New creates a kernel instance.
func New() *Kernel {
	k := &Kernel{}
	k.tickCond = sync.NewCond(&k.mu)
	return k
}

// NewEndpoint allocates a new endpoint and returns a capability for it.
func (k *Kernel) NewEndpoint(rights Rights) Capability {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.endpointCount >= maxEndpoints {
		return Capability{}
	}
	ep := k.endpointCount
	k.endpointCount++
	k.endpoints[ep] = endpointState{ch: make(chan Message, mailboxSlots)}
	return Capability{ep: ep, rights: rights}
}

// AddTask registers a task and returns its ID.
func (k *Kernel) AddTask(t Task) TaskID {
	k.mu.Lock()
	if k.taskCount >= maxTasks {
		k.mu.Unlock()
		return 0
	}
	id := k.taskCount
	k.taskCount++
	k.tasks[id] = taskState{task: t}
	k.mu.Unlock()

	if t != nil {
		go t.Run(&Context{k: k, taskID: id})
	}
	return id
}

func (k *Kernel) send(from Endpoint, to Endpoint, kind uint16, payload []byte, xfer Capability) SendResult {
	k.mu.Lock()
	if to >= k.endpointCount || k.endpoints[to].ch == nil {
		k.mu.Unlock()
		return SendErrNoEndpoint
	}
	ch := k.endpoints[to].ch
	k.mu.Unlock()

	if len(payload) > MaxMessageBytes {
		return SendErrPayloadTooLarge
	}

	var msg Message
	msg.From = from
	msg.To = to
	msg.Kind = kind
	msg.Len = uint16(len(payload))
	copy(msg.Data[:], payload)
	msg.Cap = xfer

	select {
	case ch <- msg:
		return SendOK
	default:
		return SendErrQueueFull
	}
}

// TickTo broadcasts a new tick value to tick-waiters.
func (k *Kernel) TickTo(seq uint64) {
	k.mu.Lock()
	if seq <= k.tick {
		k.mu.Unlock()
		return
	}
	k.tick = seq
	k.tickCond.Broadcast()
	k.mu.Unlock()
}

func (k *Kernel) nowTick() uint64 {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.tick
}

func (k *Kernel) waitTick(after uint64) uint64 {
	k.mu.Lock()
	for k.tick <= after {
		k.tickCond.Wait()
	}
	seq := k.tick
	k.mu.Unlock()
	return seq
}
