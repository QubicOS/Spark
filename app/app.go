package app

import (
	"spark/hal"
	"spark/sparkos/kernel"
	"spark/sparkos/services/logger"
	"spark/sparkos/services/term"
	timesvc "spark/sparkos/services/time"
	"spark/sparkos/services/ui"
	"spark/sparkos/tasks/termdemo"
)

type system struct {
	k *kernel.Kernel
}

type Config struct {
	TermDemo bool
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

	k.AddTask(logger.New(h.Logger(), logEP.Restrict(kernel.RightRecv)))
	k.AddTask(timesvc.New(timeEP))

	if cfg.TermDemo {
		k.AddTask(term.New(h.Display(), termEP.Restrict(kernel.RightRecv)))
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
