package kernel

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

// Task is a cooperative unit of execution.
type Task interface {
	Step(*Context)
}

type endpointState struct {
	q        mailbox
	waitMask uint32
}

type taskState struct {
	task     Task
	runnable bool
	waiting  Endpoint
}

// Kernel is a minimal cooperative scheduler plus IPC router.
type Kernel struct {
	endpoints     [maxEndpoints]endpointState
	endpointCount Endpoint

	tasks     [maxTasks]taskState
	taskCount TaskID

	rr TaskID

	tickWaitMask uint32
}

// New creates a kernel instance.
func New() *Kernel {
	return &Kernel{}
}

// NewEndpoint allocates a new endpoint and returns a capability for it.
func (k *Kernel) NewEndpoint(rights Rights) Capability {
	if k.endpointCount >= maxEndpoints {
		return Capability{}
	}
	ep := k.endpointCount
	k.endpointCount++
	return Capability{ep: ep, rights: rights}
}

// AddTask registers a task and returns its ID.
func (k *Kernel) AddTask(t Task) TaskID {
	if k.taskCount >= maxTasks {
		return 0
	}
	id := k.taskCount
	k.taskCount++
	k.tasks[id] = taskState{task: t, runnable: true}
	return id
}

// Step runs at most one runnable task step.
func (k *Kernel) Step() {
	if k.taskCount == 0 {
		return
	}

	for i := TaskID(0); i < k.taskCount; i++ {
		id := (k.rr + i) % k.taskCount
		st := &k.tasks[id]
		if st.task == nil || !st.runnable {
			continue
		}

		k.rr = (id + 1) % k.taskCount
		ctx := &Context{k: k, taskID: id}
		st.task.Step(ctx)

		if ctx.blocked {
			st.runnable = false
			if ctx.blockOnTick {
				k.tickWaitMask |= 1 << id
			} else {
				st.waiting = ctx.blockOn
				if st.waiting < k.endpointCount {
					k.endpoints[st.waiting].waitMask |= 1 << id
				}
			}
		}
		return
	}
}

// Tick wakes tasks blocked via Context.BlockOnTick.
func (k *Kernel) Tick() {
	wait := k.tickWaitMask
	if wait == 0 {
		return
	}

	for tid := TaskID(0); tid < k.taskCount; tid++ {
		if wait&(1<<tid) == 0 {
			continue
		}
		k.tasks[tid].runnable = true
	}
	k.tickWaitMask = 0
}

func (k *Kernel) send(from Endpoint, to Endpoint, kind uint16, payload []byte, xfer Capability) SendResult {
	if to >= k.endpointCount {
		return SendErrNoEndpoint
	}
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

	ep := &k.endpoints[to]
	if !ep.q.push(msg) {
		return SendErrQueueFull
	}

	wait := ep.waitMask
	if wait == 0 {
		return SendOK
	}

	for tid := TaskID(0); tid < k.taskCount; tid++ {
		if wait&(1<<tid) == 0 {
			continue
		}
		k.tasks[tid].runnable = true
		ep.waitMask &^= 1 << tid
	}
	return SendOK
}

func (k *Kernel) recv(to Endpoint) (Message, bool) {
	if to >= k.endpointCount {
		return Message{}, false
	}
	return k.endpoints[to].q.pop()
}
