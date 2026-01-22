//go:build tinygo && baremetal && picocalc

package kbdprobe

import (
	"fmt"
	"machine"

	termclient "spark/sparkos/client/term"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

const (
	kbdAddr uint16 = 0x1F
	kbdCmd         = 0x09
)

// Task probes the PicoCalc I2C keyboard controller and prints status to the terminal.
// It is a bring-up tool to verify I2C connectivity when input doesn't work.
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

	i2c := machine.I2C1
	if i2c == nil {
		_ = termclient.WriteString(ctx, t.termCap, "kbdprobe: I2C1 nil\n")
		_ = ctx.SendToCapResult(t.termCap, uint16(proto.MsgTermRefresh), nil, kernel.Capability{})
		return
	}
	if err := i2c.Configure(machine.I2CConfig{SCL: machine.GP7, SDA: machine.GP6}); err != nil {
		_ = termclient.WriteString(ctx, t.termCap, "kbdprobe: I2C1 cfg err: "+err.Error()+"\n")
		_ = ctx.SendToCapResult(t.termCap, uint16(proto.MsgTermRefresh), nil, kernel.Capability{})
		return
	}

	write := [1]byte{kbdCmd}
	read := [2]byte{}
	okCount := 0
	errCount := 0

	last := ctx.NowTick()
	for {
		last = ctx.WaitTick(last)
		if last%200 != 0 {
			continue
		}

		if err := i2c.Tx(kbdAddr, write[:], read[:]); err != nil {
			errCount++
		} else {
			okCount++
		}

		if last%1000 == 0 {
			_ = termclient.WriteString(
				ctx,
				t.termCap,
				fmt.Sprintf("kbdprobe: ok=%d err=%d last=%02x %02x\n", okCount, errCount, read[0], read[1]),
			)
			_ = ctx.SendToCapResult(t.termCap, uint16(proto.MsgTermRefresh), nil, kernel.Capability{})
		}
	}
}

