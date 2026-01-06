//go:build !spark_vi

package vi

import (
	"spark/hal"
	"spark/sparkos/kernel"
)

// Task is a no-op placeholder for the Spark VI task.
//
// The real implementation is gated behind the `spark_vi` build tag.
type Task struct {
	disp   hal.Display
	ep     kernel.Capability
	vfsCap kernel.Capability
}

func New(disp hal.Display, ep kernel.Capability, vfsCap kernel.Capability) *Task {
	return &Task{disp: disp, ep: ep, vfsCap: vfsCap}
}

func (t *Task) Run(ctx *kernel.Context) {
	ch, ok := ctx.RecvChan(t.ep)
	if !ok {
		return
	}
	for range ch {
	}
}
