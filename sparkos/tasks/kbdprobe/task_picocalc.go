//go:build tinygo && baremetal && picocalc

package kbdprobe

import (
	"fmt"
	"machine"
	"strings"

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

	machine.GP6.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
	machine.GP7.Configure(machine.PinConfig{Mode: machine.PinInputPullup})

	write := [1]byte{kbdCmd}
	read1 := [2]byte{}
	read0 := [2]byte{}
	ok1, err1 := 0, 0
	ok0, err0 := 0, 0
	var scanOnce bool

	last := ctx.NowTick()
	for {
		last = ctx.WaitTick(last)
		if last%200 != 0 {
			continue
		}

		if !scanOnce && last > 1000 {
			scanOnce = true
			_ = termclient.WriteString(ctx, t.termCap, "kbdprobe: scanning I2C...\n")
			_ = ctx.SendToCapResult(t.termCap, uint16(proto.MsgTermRefresh), nil, kernel.Capability{})
			_ = t.scanAndReport(ctx, "I2C1", machine.I2C1)
			_ = t.scanAndReport(ctx, "I2C0", machine.I2C0)
			_ = termclient.WriteString(ctx, t.termCap, "kbdprobe: scan done\n")
			_ = ctx.SendToCapResult(t.termCap, uint16(proto.MsgTermRefresh), nil, kernel.Capability{})
		}

		if last%1000 == 0 {
			if machine.I2C1 != nil {
				if err := machine.I2C1.Configure(machine.I2CConfig{SCL: machine.GP7, SDA: machine.GP6}); err == nil {
					if err := machine.I2C1.Tx(kbdAddr, write[:], read1[:]); err != nil {
						err1++
					} else {
						ok1++
					}
				} else {
					err1++
				}
			}
			if machine.I2C0 != nil {
				if err := machine.I2C0.Configure(machine.I2CConfig{SCL: machine.GP7, SDA: machine.GP6}); err == nil {
					if err := machine.I2C0.Tx(kbdAddr, write[:], read0[:]); err != nil {
						err0++
					} else {
						ok0++
					}
				} else {
					err0++
				}
			}
			_ = termclient.WriteString(
				ctx,
				t.termCap,
				fmt.Sprintf(
					"kbdprobe: I2C1 ok=%d err=%d last=%02x %02x | I2C0 ok=%d err=%d last=%02x %02x\n",
					ok1, err1, read1[0], read1[1],
					ok0, err0, read0[0], read0[1],
				),
			)
			_ = ctx.SendToCapResult(t.termCap, uint16(proto.MsgTermRefresh), nil, kernel.Capability{})
		}
	}
}

func (t *Task) scanAndReport(ctx *kernel.Context, name string, bus *machine.I2C) error {
	if ctx == nil || !t.termCap.Valid() {
		return nil
	}
	if bus == nil {
		_ = termclient.WriteString(ctx, t.termCap, "kbdprobe: "+name+" nil\n")
		_ = ctx.SendToCapResult(t.termCap, uint16(proto.MsgTermRefresh), nil, kernel.Capability{})
		return nil
	}
	if err := bus.Configure(machine.I2CConfig{SCL: machine.GP7, SDA: machine.GP6}); err != nil {
		_ = termclient.WriteString(ctx, t.termCap, "kbdprobe: "+name+" cfg err: "+err.Error()+"\n")
		_ = ctx.SendToCapResult(t.termCap, uint16(proto.MsgTermRefresh), nil, kernel.Capability{})
		return nil
	}

	var found []string
	tmp := []byte{0}
	for addr := uint16(0x03); addr <= 0x77; addr++ {
		// Best-effort probe: attempt 1-byte read.
		if err := bus.Tx(addr, nil, tmp); err == nil {
			found = append(found, fmt.Sprintf("0x%02x", addr))
		}
	}
	line := "kbdprobe: " + name + " found: "
	if len(found) == 0 {
		line += "(none)\n"
	} else {
		line += strings.Join(found, " ") + "\n"
	}
	_ = termclient.WriteString(ctx, t.termCap, line)
	_ = ctx.SendToCapResult(t.termCap, uint16(proto.MsgTermRefresh), nil, kernel.Capability{})
	return nil
}
