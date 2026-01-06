package consolemux

import (
	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

const interruptByte = 0x07 // Ctrl+G

type Service struct {
	inCap  kernel.Capability
	ctlCap kernel.Capability

	shellCap   kernel.Capability
	rtdemoCap  kernel.Capability
	rtvoxelCap kernel.Capability
	viCap      kernel.Capability
	termCap    kernel.Capability

	activeApp proto.AppID
	appActive bool
}

func New(inCap, ctlCap, shellCap, rtdemoCap, rtvoxelCap, viCap, termCap kernel.Capability) *Service {
	return &Service{
		inCap:      inCap,
		ctlCap:     ctlCap,
		shellCap:   shellCap,
		rtdemoCap:  rtdemoCap,
		rtvoxelCap: rtvoxelCap,
		viCap:      viCap,
		termCap:    termCap,
		activeApp:  proto.AppRTDemo,
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
		case proto.MsgAppSelect:
			appID, arg, ok := proto.DecodeAppSelectPayload(msg.Data[:msg.Len])
			if !ok {
				continue
			}
			s.handleAppSelect(ctx, appID, arg)
		}
	}
}

func (s *Service) handleInput(ctx *kernel.Context, b []byte) {
	if s.appActive {
		s.flushInput(ctx, b)
		return
	}

	start := 0
	for i := 0; i < len(b); i++ {
		switch b[i] {
		case interruptByte:
			s.flushInput(ctx, b[start:i])
			start = i + 1
			s.setActive(ctx, true)
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
		dst = s.selectedAppCap()
	}
	_ = sendWithRetry(ctx, dst, proto.MsgTermInput, b, kernel.Capability{})
}

func (s *Service) setActive(ctx *kernel.Context, active bool) {
	if active == s.appActive {
		return
	}
	if active && !s.selectedAppCap().Valid() {
		return
	}
	s.appActive = active

	appCap := s.selectedAppCap()
	var xfer kernel.Capability
	if s.appActive && s.ctlCap.Valid() {
		xfer = s.ctlCap
	}
	_ = sendWithRetry(ctx, appCap, proto.MsgAppControl, proto.AppControlPayload(active), xfer)
	_ = sendWithRetry(ctx, s.shellCap, proto.MsgAppControl, proto.AppControlPayload(!active), kernel.Capability{})
}

func (s *Service) handleAppSelect(ctx *kernel.Context, id proto.AppID, arg string) {
	appCap := s.appCapByID(id)
	if !appCap.Valid() {
		return
	}
	s.activeApp = id
	_ = sendWithRetry(ctx, appCap, proto.MsgAppSelect, proto.AppSelectPayload(id, arg), kernel.Capability{})
}

func (s *Service) selectedAppCap() kernel.Capability {
	return s.appCapByID(s.activeApp)
}

func (s *Service) appCapByID(id proto.AppID) kernel.Capability {
	switch id {
	case proto.AppRTDemo:
		return s.rtdemoCap
	case proto.AppRTVoxel:
		return s.rtvoxelCap
	case proto.AppVi:
		return s.viCap
	default:
		return kernel.Capability{}
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
