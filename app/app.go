package app

import (
	"spark/hal"
	"spark/sparkos/kernel"
	"spark/sparkos/services/logger"
	"spark/sparkos/services/shell"
	"spark/sparkos/services/term"
	"spark/sparkos/services/termkbd"
	timesvc "spark/sparkos/services/time"
	"spark/sparkos/services/ui"
	"spark/sparkos/services/vfs"
	"spark/sparkos/tasks/bootmsg"
	"spark/sparkos/tasks/kbdprobe"
	"spark/sparkos/tasks/termdemo"
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
	installPanicHandler(h)

	if cfg.Shell {
		bootDiagStart(h)
		bootScreen(h, "init: kernel")
	}

	logEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	timeEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	termEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	shellEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	vfsEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	audioEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	gpioEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	serialEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)

	k.AddTask(logger.New(h.Logger(), logEP.Restrict(kernel.RightRecv)))
	k.AddTask(timesvc.New(timeEP))
	k.AddTask(vfs.New(h.Flash(), vfsEP.Restrict(kernel.RightRecv)))
	_ = audioEP
	_ = gpioEP
	_ = serialEP

	if cfg.Shell {
		bootScreen(h, "init: term")
		k.AddTask(term.New(h.Display(), termEP.Restrict(kernel.RightRecv)))
		k.AddTask(bootmsg.New(termEP.Restrict(kernel.RightSend)))
		k.AddTask(kbdprobe.New(termEP.Restrict(kernel.RightSend)))
		bootScreen(h, "init: termkbd")
		k.AddTask(termkbd.NewInput(h.Input(), shellEP.Restrict(kernel.RightSend)))
		k.AddTask(shell.New(
			shellEP.Restrict(kernel.RightRecv),
			termEP.Restrict(kernel.RightSend),
			logEP.Restrict(kernel.RightSend),
			kernel.Capability{}, // no VFS
			timeEP.Restrict(kernel.RightSend),
			kernel.Capability{}, // no consolemux
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
