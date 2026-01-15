package app

import (
	"spark/hal"
	"spark/sparkos/kernel"
	"spark/sparkos/services/appmgr"
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

	logEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	timeEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	termEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	shellEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	vfsEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	audioEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	gpioEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	serialEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	muxEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	rtdemoEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	rtvoxelEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	imgviewEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	viEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	mcEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	hexEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	vectorEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	snakeEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	tetrisEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	calendarEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	todoEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	archiveEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	teaEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	basicEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	gpioscopeEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	fbtestEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	serialtermEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	usersEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)

	rtdemoProxyEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	rtvoxelProxyEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	imgviewProxyEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	hexProxyEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	snakeProxyEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	tetrisProxyEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	calendarProxyEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	todoProxyEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	archiveProxyEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	viProxyEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	mcProxyEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	vectorProxyEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	teaProxyEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	basicProxyEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	gpioscopeProxyEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	fbtestProxyEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	serialtermProxyEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	usersProxyEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)

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
		k.AddTask(term.New(h.Display(), termEP.Restrict(kernel.RightRecv)))
		k.AddTask(appmgr.New(
			h.Display(),
			vfsEP.Restrict(kernel.RightSend),
			audioEP.Restrict(kernel.RightSend),
			timeEP.Restrict(kernel.RightSend),
			gpioEP.Restrict(kernel.RightSend),
			serialEP.Restrict(kernel.RightSend),
			rtdemoProxyEP.Restrict(kernel.RightRecv),
			rtvoxelProxyEP.Restrict(kernel.RightRecv),
			imgviewProxyEP.Restrict(kernel.RightRecv),
			hexProxyEP.Restrict(kernel.RightRecv),
			snakeProxyEP.Restrict(kernel.RightRecv),
			tetrisProxyEP.Restrict(kernel.RightRecv),
			calendarProxyEP.Restrict(kernel.RightRecv),
			todoProxyEP.Restrict(kernel.RightRecv),
			archiveProxyEP.Restrict(kernel.RightRecv),
			viProxyEP.Restrict(kernel.RightRecv),
			mcProxyEP.Restrict(kernel.RightRecv),
			vectorProxyEP.Restrict(kernel.RightRecv),
			teaProxyEP.Restrict(kernel.RightRecv),
			basicProxyEP.Restrict(kernel.RightRecv),
			gpioscopeProxyEP.Restrict(kernel.RightRecv),
			fbtestProxyEP.Restrict(kernel.RightRecv),
			serialtermProxyEP.Restrict(kernel.RightRecv),
			usersProxyEP.Restrict(kernel.RightRecv),
			rtdemoEP.Restrict(kernel.RightSend),
			rtvoxelEP.Restrict(kernel.RightSend),
			imgviewEP.Restrict(kernel.RightSend),
			hexEP.Restrict(kernel.RightSend),
			snakeEP.Restrict(kernel.RightSend),
			tetrisEP.Restrict(kernel.RightSend),
			calendarEP.Restrict(kernel.RightSend),
			todoEP.Restrict(kernel.RightSend),
			archiveEP.Restrict(kernel.RightSend),
			viEP.Restrict(kernel.RightSend),
			mcEP.Restrict(kernel.RightSend),
			vectorEP.Restrict(kernel.RightSend),
			teaEP.Restrict(kernel.RightSend),
			basicEP.Restrict(kernel.RightSend),
			gpioscopeEP.Restrict(kernel.RightSend),
			fbtestEP.Restrict(kernel.RightSend),
			serialtermEP.Restrict(kernel.RightSend),
			usersEP.Restrict(kernel.RightSend),
			rtdemoEP.Restrict(kernel.RightRecv),
			rtvoxelEP.Restrict(kernel.RightRecv),
			imgviewEP.Restrict(kernel.RightRecv),
			hexEP.Restrict(kernel.RightRecv),
			snakeEP.Restrict(kernel.RightRecv),
			tetrisEP.Restrict(kernel.RightRecv),
			calendarEP.Restrict(kernel.RightRecv),
			todoEP.Restrict(kernel.RightRecv),
			archiveEP.Restrict(kernel.RightRecv),
			viEP.Restrict(kernel.RightRecv),
			mcEP.Restrict(kernel.RightRecv),
			vectorEP.Restrict(kernel.RightRecv),
			teaEP.Restrict(kernel.RightRecv),
			basicEP.Restrict(kernel.RightRecv),
			gpioscopeEP.Restrict(kernel.RightRecv),
			fbtestEP.Restrict(kernel.RightRecv),
			serialtermEP.Restrict(kernel.RightRecv),
			usersEP.Restrict(kernel.RightRecv),
		))
		k.AddTask(consolemux.New(
			muxEP.Restrict(kernel.RightRecv),
			muxEP.Restrict(kernel.RightSend),
			shellEP.Restrict(kernel.RightSend),
			rtdemoProxyEP.Restrict(kernel.RightSend),
			rtvoxelProxyEP.Restrict(kernel.RightSend),
			imgviewProxyEP.Restrict(kernel.RightSend),
			viProxyEP.Restrict(kernel.RightSend),
			mcProxyEP.Restrict(kernel.RightSend),
			hexProxyEP.Restrict(kernel.RightSend),
			vectorProxyEP.Restrict(kernel.RightSend),
			snakeProxyEP.Restrict(kernel.RightSend),
			tetrisProxyEP.Restrict(kernel.RightSend),
			calendarProxyEP.Restrict(kernel.RightSend),
			todoProxyEP.Restrict(kernel.RightSend),
			archiveProxyEP.Restrict(kernel.RightSend),
			teaProxyEP.Restrict(kernel.RightSend),
			basicProxyEP.Restrict(kernel.RightSend),
			gpioscopeProxyEP.Restrict(kernel.RightSend),
			fbtestProxyEP.Restrict(kernel.RightSend),
			serialtermProxyEP.Restrict(kernel.RightSend),
			usersProxyEP.Restrict(kernel.RightSend),
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
