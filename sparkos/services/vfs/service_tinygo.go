//go:build tinygo

package vfs

import (
	"spark/hal"
	"spark/sparkos/kernel"
)

type Service struct {
	_     [0]func()
	inCap kernel.Capability
	flash hal.Flash
}

func New(flash hal.Flash, inCap kernel.Capability) *Service {
	return &Service{flash: flash, inCap: inCap}
}

func (s *Service) Run(ctx *kernel.Context) {
	_, ok := ctx.RecvChan(s.inCap)
	if !ok {
		return
	}
	select {}
}
