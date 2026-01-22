package bootmsg

import (
	"fmt"

	termclient "spark/sparkos/client/term"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

// Task prints a small boot message to the terminal.
type Task struct {
	termCap kernel.Capability
}

// New returns a boot message task.
func New(termCap kernel.Capability) *Task {
	return &Task{termCap: termCap}
}

func (t *Task) Run(ctx *kernel.Context) {
	if ctx == nil || !t.termCap.Valid() {
		return
	}
	_ = termclient.Write(ctx, t.termCap, []byte(fmt.Sprintf("boot: shell=%v\n", true)))
	_ = ctx.SendToCapResult(t.termCap, uint16(proto.MsgTermRefresh), nil, kernel.Capability{})
}
