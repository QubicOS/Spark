package app

import (
	"spark/hal"
	"spark/sparkos/kernel"
	"spark/sparkos/services/consolemux"
	"spark/sparkos/services/logger"
	"spark/sparkos/services/shell"
	"spark/sparkos/services/term"
	"spark/sparkos/services/termkbd"
	timesvc "spark/sparkos/services/time"
	"spark/sparkos/services/ui"
	"spark/sparkos/services/vfs"
	"spark/sparkos/tasks/rtdemo"
	"spark/sparkos/tasks/rtvoxel"
	"spark/sparkos/tasks/termdemo"
	"spark/sparkos/tasks/vi"
)

type system struct {
	k *kernel.Kernel
}

type Config struct {
	TermDemo bool
	Shell    bool
}

// New initializes and starts the OS with default config.
func New(h hal.HAL) func() error {
	_ = newSystem(h, Config{})
	return func() error { return nil }
}

// Run starts the OS and blocks forever (TinyGo/native entrypoint).
func Run(h hal.HAL) {
	_ = New(h)
	select {}
}

func NewWithConfig(h hal.HAL, cfg Config) func() error {
	_ = newSystem(h, cfg)
	return func() error { return nil }
}

func RunWithConfig(h hal.HAL, cfg Config) {
	_ = NewWithConfig(h, cfg)
	select {}
}

func newSystem(h hal.HAL, cfg Config) *system {
	k := kernel.New()

	logEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	timeEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	termEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	shellEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	vfsEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	muxEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	rtdemoEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	rtvoxelEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	viEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)

	k.AddTask(logger.New(h.Logger(), logEP.Restrict(kernel.RightRecv)))
	k.AddTask(timesvc.New(timeEP))
	k.AddTask(vfs.New(h.Flash(), vfsEP.Restrict(kernel.RightRecv)))

	if cfg.Shell {
		k.AddTask(term.New(h.Display(), termEP.Restrict(kernel.RightRecv)))
		k.AddTask(rtdemo.New(h.Display(), rtdemoEP.Restrict(kernel.RightRecv)))
		k.AddTask(rtvoxel.New(h.Display(), rtvoxelEP.Restrict(kernel.RightRecv)))
		k.AddTask(vi.New(h.Display(), viEP.Restrict(kernel.RightRecv), vfsEP.Restrict(kernel.RightSend)))
		k.AddTask(consolemux.New(
			muxEP.Restrict(kernel.RightRecv),
			muxEP.Restrict(kernel.RightSend),
			shellEP.Restrict(kernel.RightSend),
			rtdemoEP.Restrict(kernel.RightSend),
			rtvoxelEP.Restrict(kernel.RightSend),
			viEP.Restrict(kernel.RightSend),
			termEP.Restrict(kernel.RightSend),
		))
		k.AddTask(termkbd.NewInput(h.Input(), muxEP.Restrict(kernel.RightSend)))
		k.AddTask(shell.New(
			shellEP.Restrict(kernel.RightRecv),
			termEP.Restrict(kernel.RightSend),
			logEP.Restrict(kernel.RightSend),
			vfsEP.Restrict(kernel.RightSend),
			timeEP.Restrict(kernel.RightSend),
			muxEP.Restrict(kernel.RightSend),
		))
	} else if cfg.TermDemo {
		k.AddTask(term.New(h.Display(), termEP.Restrict(kernel.RightRecv)))
		k.AddTask(termkbd.New(h.Input(), termEP.Restrict(kernel.RightSend)))
		k.AddTask(termdemo.New(timeEP.Restrict(kernel.RightSend), termEP.Restrict(kernel.RightSend)))
	} else {
		k.AddTask(ui.New(h.Display(), h.Input()))
	}

	if ht := h.Time(); ht != nil {
		if ch := ht.Ticks(); ch != nil {
			go func() {
				for seq := range ch {
					k.TickTo(seq)
				}
			}()
		}
	}

	return &system{k: k}
}
