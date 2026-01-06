package consolemux

import (
	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

const interruptByte = 0x07 // Ctrl+G

type Service struct {
	inCap  kernel.Capability
	ctlCap kernel.Capability

	shellCap kernel.Capability
	appCap   kernel.Capability
	termCap  kernel.Capability

	appActive bool
}

func New(inCap, ctlCap, shellCap, appCap, termCap kernel.Capability) *Service {
	return &Service{
		inCap:    inCap,
		ctlCap:   ctlCap,
		shellCap: shellCap,
		appCap:   appCap,
		termCap:  termCap,
	}
}

func (s *Service) Run(ctx *kernel.Context) {
	ch, ok := ctx.RecvChan(s.inCap)
	if !ok {
		return
	}

	for msg := range ch {
		switch proto.Kind(msg.Kind) {
		case proto.MsgTermInput:
			s.handleInput(ctx, msg.Data[:msg.Len])
		case proto.MsgAppControl:
			active, ok := proto.DecodeAppControlPayload(msg.Data[:msg.Len])
			if !ok {
				continue
			}
			s.setActive(ctx, active)
		}
	}
}

func (s *Service) handleInput(ctx *kernel.Context, b []byte) {
	start := 0
	for i := 0; i < len(b); i++ {
		switch b[i] {
		case interruptByte:
			s.flushInput(ctx, b[start:i])
			start = i + 1
			s.setActive(ctx, !s.appActive)
		}
	}
	s.flushInput(ctx, b[start:])
}

func (s *Service) flushInput(ctx *kernel.Context, b []byte) {
	if len(b) == 0 {
		return
	}
	dst := s.shellCap
	if s.appActive {
		dst = s.appCap
	}
	_ = sendWithRetry(ctx, dst, proto.MsgTermInput, b, kernel.Capability{})
}

func (s *Service) setActive(ctx *kernel.Context, active bool) {
	if active == s.appActive {
		return
	}
	s.appActive = active

	var xfer kernel.Capability
	if s.appActive && s.ctlCap.Valid() {
		xfer = s.ctlCap
	}
	_ = sendWithRetry(ctx, s.appCap, proto.MsgAppControl, proto.AppControlPayload(active), xfer)
	if !s.appActive {
		_ = sendWithRetry(ctx, s.termCap, proto.MsgTermRefresh, nil, kernel.Capability{})
	}
}

func sendWithRetry(ctx *kernel.Context, toCap kernel.Capability, kind proto.Kind, payload []byte, xfer kernel.Capability) error {
	if !toCap.Valid() {
		return nil
	}
	if len(payload) == 0 {
		for {
			res := ctx.SendToCapResult(toCap, uint16(kind), nil, xfer)
			switch res {
			case kernel.SendOK:
				return nil
			case kernel.SendErrQueueFull:
				ctx.BlockOnTick()
			default:
				return nil
			}
		}
	}

	for len(payload) > 0 {
		chunk := payload
		if len(chunk) > kernel.MaxMessageBytes {
			chunk = chunk[:kernel.MaxMessageBytes]
		}

		for {
			res := ctx.SendToCapResult(toCap, uint16(kind), chunk, xfer)
			switch res {
			case kernel.SendOK:
				payload = payload[len(chunk):]
				goto nextChunk
			case kernel.SendErrQueueFull:
				ctx.BlockOnTick()
			default:
				return nil
			}
		}
	nextChunk:
	}
	return nil
}
