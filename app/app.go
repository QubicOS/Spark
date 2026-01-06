package app

import (
	"spark/hal"
	"spark/sparkos/kernel"
	"spark/sparkos/services/logger"
	timesvc "spark/sparkos/services/time"
	"spark/sparkos/services/ui"
)

type system struct {
	k *kernel.Kernel
}

// New initializes the OS and returns a single-step function.
//
// The stepper is intended for the host emulator game loop.
func New(h hal.HAL) func() error {
	_ = newSystem(h)
	return func() error { return nil }
}

// NewWithBudget initializes the OS and returns a single-step function.
//
// The budget controls how many kernel steps are executed per call.
func NewWithBudget(h hal.HAL, budget int) func() error {
	_ = budget
	return New(h)
}

// Run starts the OS and blocks forever (TinyGo/native entrypoint).
func Run(h hal.HAL) {
	_ = New(h)
	select {}
}

func newSystem(h hal.HAL) *system {
	k := kernel.New()

	logEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	timeEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)

	k.AddTask(logger.New(h.Logger(), logEP.Restrict(kernel.RightRecv)))
	k.AddTask(timesvc.New(timeEP))
	k.AddTask(ui.New(h.Display(), h.Input()))

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
