//go:build !tinygo

package kbdprobe

import "spark/sparkos/kernel"

type Task struct{}

func New(_ kernel.Capability) *Task { return &Task{} }

func (t *Task) Run(ctx *kernel.Context) {
	_ = ctx
}

