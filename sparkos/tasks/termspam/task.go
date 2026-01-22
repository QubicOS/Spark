package termspam

import (
	"fmt"

	termclient "spark/sparkos/client/term"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

// Task periodically writes a small message to the terminal.
// It is intended for baremetal bring-up debugging when shell input isn't working.
type Task struct {
	termCap kernel.Capability
}

func New(termCap kernel.Capability) *Task {
	return &Task{termCap: termCap}
}

func (t *Task) Run(ctx *kernel.Context) {
	if ctx == nil || !t.termCap.Valid() {
		return
	}

	seq := uint64(0)
	last := ctx.NowTick()
	for {
		last = ctx.WaitTick(last + 1000)
		seq++

		_ = termclient.WriteString(ctx, t.termCap, fmt.Sprintf("termspam: %d\n", seq))
		_ = ctx.SendToCapResult(t.termCap, uint16(proto.MsgTermRefresh), nil, kernel.Capability{})
	}
}
