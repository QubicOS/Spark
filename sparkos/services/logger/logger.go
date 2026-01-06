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

func (s *Service) Step(ctx *kernel.Context) {
	msg, ok := ctx.Recv(s.ep)
	if !ok {
		return
	}
	if s.log == nil {
		return
	}
	if msg.Kind != uint16(proto.MsgLogLine) {
		return
	}
	s.log.WriteLineBytes(msg.Data[:msg.Len])
}
