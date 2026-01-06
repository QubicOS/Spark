package timesvc

import (
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
	ep kernel.Capability

	now      uint64
	sleepers [maxSleepers]sleeper
}

func New(ep kernel.Capability) *Service {
	return &Service{ep: ep}
}

func (s *Service) Run(ctx *kernel.Context) {
	reqCh, ok := ctx.RecvChan(s.ep)
	if !ok {
		return
	}

	tickCh := make(chan uint64, 128)
	go func() {
		last := ctx.NowTick()
		for {
			last = ctx.WaitTick(last)
			select {
			case tickCh <- last:
			default:
			}
		}
	}()

	for {
		select {
		case now := <-tickCh:
			s.now = now
			s.wakeReady(ctx)

		case msg := <-reqCh:
			if msg.Kind != uint16(proto.MsgSleep) {
				continue
			}
			if !msg.Cap.Valid() {
				continue
			}

			requestID, dt, ok := proto.DecodeSleepPayload(msg.Data[:msg.Len])
			if !ok {
				payload := proto.ErrorPayload(
					proto.ErrBadMessage,
					proto.MsgSleep,
					proto.ErrorDetailWithRequestID(0, nil),
				)
				_ = ctx.Send(s.ep, msg.Cap, uint16(proto.MsgError), payload)
				continue
			}
			if dt == 0 {
				_ = ctx.Send(s.ep, msg.Cap, uint16(proto.MsgWake), proto.WakePayload(requestID))
				continue
			}
			if ok := s.schedule(s.now+uint64(dt), requestID, msg.Cap); !ok {
				payload := proto.ErrorPayload(
					proto.ErrOverflow,
					proto.MsgSleep,
					proto.ErrorDetailWithRequestID(requestID, nil),
				)
				_ = ctx.Send(s.ep, msg.Cap, uint16(proto.MsgError), payload)
				continue
			}
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
