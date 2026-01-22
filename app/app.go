package app

import (
	"spark/hal"
	"spark/sparkos/kernel"
	audiosvc "spark/sparkos/services/audio"
	"spark/sparkos/services/consolemux"
	gpiosvc "spark/sparkos/services/gpio"
	"spark/sparkos/services/logger"
	serialsvc "spark/sparkos/services/serial"
	"spark/sparkos/services/shell"
	"spark/sparkos/services/term"
	"spark/sparkos/services/termkbd"
	timesvc "spark/sparkos/services/time"
	"spark/sparkos/services/ui"
	"spark/sparkos/services/vfs"
	"spark/sparkos/tasks/bootmsg"
	"spark/sparkos/tasks/termdemo"
	vectortask "spark/sparkos/tasks/vector"
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
	muxEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)

	// Minimal set of foreground apps to keep RAM usage down.
	vectorEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	vectorProxyEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)

	k.AddTask(logger.New(h.Logger(), logEP.Restrict(kernel.RightRecv)))
	k.AddTask(timesvc.New(timeEP))
	k.AddTask(vfs.New(h.Flash(), vfsEP.Restrict(kernel.RightRecv)))
	k.AddTask(gpiosvc.New(h.GPIO(), gpioEP.Restrict(kernel.RightRecv)))
	k.AddTask(serialsvc.New(h.Serial(), serialEP.Restrict(kernel.RightRecv)))
	if ha := h.Audio(); ha != nil {
		k.AddTask(audiosvc.New(audioEP.Restrict(kernel.RightRecv), vfsEP.Restrict(kernel.RightSend), ha.PWM()))
	} else {
		k.AddTask(audiosvc.New(audioEP.Restrict(kernel.RightRecv), vfsEP.Restrict(kernel.RightSend), nil))
	}

	if cfg.Shell {
		bootScreen(h, "init: term")
		k.AddTask(term.New(h.Display(), termEP.Restrict(kernel.RightRecv)))
		k.AddTask(bootmsg.New(termEP.Restrict(kernel.RightSend)))
		bootScreen(h, "init: termkbd/shell")
		k.AddTask(termkbd.NewInput(h.Input(), muxEP.Restrict(kernel.RightSend)))

		bootScreen(h, "init: vector")
		k.AddTask(vectortask.New(h.Display(), vectorEP.Restrict(kernel.RightRecv), kernel.Capability{}))

		bootScreen(h, "init: consolemux")
		k.AddTask(consolemux.New(
			muxEP.Restrict(kernel.RightRecv),
			muxEP.Restrict(kernel.RightSend),
			shellEP.Restrict(kernel.RightSend),
			kernel.Capability{}, // rtdemo proxy
			kernel.Capability{}, // rtvoxel proxy
			kernel.Capability{}, // imgview proxy
			kernel.Capability{}, // vi proxy
			kernel.Capability{}, // mc proxy
			kernel.Capability{}, // hex proxy
			vectorProxyEP.Restrict(kernel.RightSend),
			kernel.Capability{}, // snake proxy
			kernel.Capability{}, // tetris proxy
			kernel.Capability{}, // calendar proxy
			kernel.Capability{}, // todo proxy
			kernel.Capability{}, // archive proxy
			kernel.Capability{}, // tea proxy
			kernel.Capability{}, // basic proxy
			kernel.Capability{}, // rf analyzer proxy
			kernel.Capability{}, // gpio scope proxy
			kernel.Capability{}, // fbtest proxy
			kernel.Capability{}, // serialterm proxy
			kernel.Capability{}, // users proxy
			kernel.Capability{}, // donut proxy
			termEP.Restrict(kernel.RightSend),
		))
		k.AddTask(shell.New(
			shellEP.Restrict(kernel.RightRecv),
			termEP.Restrict(kernel.RightSend),
			logEP.Restrict(kernel.RightSend),
			kernel.Capability{}, // no VFS
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
