package app

import (
	"spark/hal"
	"spark/sparkos/kernel"
	"spark/sparkos/services/logger"
	timesvc "spark/sparkos/services/time"
	"spark/sparkos/services/ui"
)

const defaultStepBudget = 64

type system struct {
	k *kernel.Kernel
}

// New initializes the OS and returns a single-step function.
//
// The stepper is intended for the host emulator game loop.
func New(h hal.HAL) func() error {
	return NewWithBudget(h, defaultStepBudget)
}

// NewWithBudget initializes the OS and returns a single-step function.
//
// The budget controls how many kernel steps are executed per call.
func NewWithBudget(h hal.HAL, budget int) func() error {
	if budget <= 0 {
		budget = 1
	}

	sys := newSystem(h)
	return func() error {
		sys.k.Tick()
		for i := 0; i < budget; i++ {
			sys.k.Step()
		}
		return nil
	}
}

// Run starts the OS and blocks forever (TinyGo/native entrypoint).
func Run(h hal.HAL) {
	step := New(h)
	for range h.Time().Ticks() {
		_ = step()
	}
}

func newSystem(h hal.HAL) *system {
	k := kernel.New()

	logEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	timeEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)

	k.AddTask(logger.New(h.Logger(), logEP.Restrict(kernel.RightRecv)))
	k.AddTask(timesvc.New(h.Time(), timeEP))
	k.AddTask(ui.New(h.Display(), h.Input()))

	return &system{k: k}
}
