//go:build !spark_vi

package vi

import (
	"spark/hal"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

// Task is a no-op placeholder for the Spark VI task.
//
// The real implementation is gated behind the `spark_vi` build tag.
type Task struct {
	disp   hal.Display
	ep     kernel.Capability
	vfsCap kernel.Capability
}

// Enabled reports whether SparkVi is compiled into this build.
const Enabled = false

func New(disp hal.Display, ep kernel.Capability, vfsCap kernel.Capability) *Task {
	return &Task{disp: disp, ep: ep, vfsCap: vfsCap}
}

func (t *Task) Run(ctx *kernel.Context) {
	ch, ok := ctx.RecvChan(t.ep)
	if !ok {
		return
	}
	var muxCap kernel.Capability
	for msg := range ch {
		switch proto.Kind(msg.Kind) {
		case proto.MsgAppControl:
			if msg.Cap.Valid() {
				muxCap = msg.Cap
			}
			active, ok := proto.DecodeAppControlPayload(msg.Payload())
			if !ok {
				continue
			}
			if active {
				requestExit(ctx, muxCap)
			}
		}
	}
}

func requestExit(ctx *kernel.Context, muxCap kernel.Capability) {
	if !muxCap.Valid() {
		return
	}
	_ = ctx.SendToCapRetry(muxCap, uint16(proto.MsgAppControl), proto.AppControlPayload(false), kernel.Capability{}, 500)
}
