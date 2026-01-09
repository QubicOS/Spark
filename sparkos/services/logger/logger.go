package logger

import (
	"spark/hal"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

type Service struct {
	log hal.Logger
	ep  kernel.Capability
}

func New(log hal.Logger, ep kernel.Capability) *Service {
	return &Service{log: log, ep: ep}
}

func (s *Service) Run(ctx *kernel.Context) {
	ch, ok := ctx.RecvChan(s.ep)
	if !ok {
		return
	}
	for msg := range ch {
		if s.log == nil {
			continue
		}
		if msg.Kind != uint16(proto.MsgLogLine) {
			continue
		}
		s.log.WriteLineBytes(msg.Payload())
	}
}
