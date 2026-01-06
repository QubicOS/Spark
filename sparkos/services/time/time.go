package timesvc

import (
	"spark/hal"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

const maxSleepers = 32

type sleeper struct {
	inUse bool
	due   uint64
	id    uint32
	reply kernel.Capability
}

type Service struct {
	ht hal.Time

	ep kernel.Capability

	now      uint64
	sleepers [maxSleepers]sleeper
}

func New(ht hal.Time, ep kernel.Capability) *Service {
	return &Service{ht: ht, ep: ep}
}

func (s *Service) Step(ctx *kernel.Context) {
	s.drainTicks()
	s.wakeReady(ctx)

	msg, ok := ctx.TryRecv(s.ep)
	if !ok {
		return
	}
	if msg.Kind != uint16(proto.MsgSleep) {
		return
	}
	if !msg.Cap.Valid() {
		return
	}

	requestID, dt, ok := proto.DecodeSleepPayload(msg.Data[:msg.Len])
	if !ok {
		payload := proto.ErrorPayload(
			proto.ErrBadMessage,
			proto.MsgSleep,
			proto.ErrorDetailWithRequestID(0, nil),
		)
		_ = ctx.Send(s.ep, msg.Cap, uint16(proto.MsgError), payload)
		return
	}
	if dt == 0 {
		_ = ctx.Send(s.ep, msg.Cap, uint16(proto.MsgWake), proto.WakePayload(requestID))
		return
	}
	if ok := s.schedule(s.now+uint64(dt), requestID, msg.Cap); !ok {
		payload := proto.ErrorPayload(
			proto.ErrOverflow,
			proto.MsgSleep,
			proto.ErrorDetailWithRequestID(requestID, nil),
		)
		_ = ctx.Send(s.ep, msg.Cap, uint16(proto.MsgError), payload)
		return
	}
}

func (s *Service) drainTicks() {
	if s.ht == nil {
		return
	}
	ch := s.ht.Ticks()
	if ch == nil {
		return
	}
	for {
		select {
		case seq := <-ch:
			s.now = seq
		default:
			return
		}
	}
}

func (s *Service) schedule(due uint64, requestID uint32, reply kernel.Capability) bool {
	for i := range s.sleepers {
		if s.sleepers[i].inUse {
			continue
		}
		s.sleepers[i] = sleeper{inUse: true, due: due, id: requestID, reply: reply}
		return true
	}
	return false
}

func (s *Service) wakeReady(ctx *kernel.Context) {
	for i := range s.sleepers {
		sl := &s.sleepers[i]
		if !sl.inUse || sl.due > s.now {
			continue
		}
		_ = ctx.Send(s.ep, sl.reply, uint16(proto.MsgWake), proto.WakePayload(sl.id))
		*sl = sleeper{}
	}
}
