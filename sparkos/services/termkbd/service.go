package termkbd

import (
	"spark/hal"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

type Service struct {
	in      hal.Input
	termCap kernel.Capability

	events  <-chan hal.KeyEvent
	pending []byte
}

func New(in hal.Input, termCap kernel.Capability) *Service {
	return &Service{in: in, termCap: termCap}
}

func (s *Service) Run(ctx *kernel.Context) {
	if ctx == nil {
		return
	}
	if s.in == nil {
		return
	}
	kbd := s.in.Keyboard()
	if kbd == nil {
		return
	}
	s.events = kbd.Events()
	if s.events == nil {
		return
	}

	tickCh := make(chan uint64, 16)
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
		case ev := <-s.events:
			if !ev.Press {
				continue
			}
			s.pending = append(s.pending, vt100FromKey(ev)...)
			s.flush(ctx)
		case <-tickCh:
			s.flush(ctx)
		}
	}
}

func (s *Service) flush(ctx *kernel.Context) {
	if len(s.pending) == 0 {
		return
	}
	if !s.termCap.Valid() {
		s.pending = nil
		return
	}

	chunk := s.pending
	if len(chunk) > kernel.MaxMessageBytes {
		chunk = chunk[:kernel.MaxMessageBytes]
	}

	res := ctx.SendToCapResult(s.termCap, uint16(proto.MsgTermWrite), chunk, kernel.Capability{})
	switch res {
	case kernel.SendOK:
		s.pending = s.pending[len(chunk):]
	case kernel.SendErrQueueFull:
	default:
		s.pending = nil
	}
}

func vt100FromKey(ev hal.KeyEvent) []byte {
	if ev.Rune != 0 {
		return []byte(string(ev.Rune))
	}

	switch ev.Code {
	case hal.KeyEnter:
		return []byte{'\n'}
	case hal.KeyEscape:
		return []byte{0x1b}
	case hal.KeyUp:
		return []byte("\x1b[A")
	case hal.KeyDown:
		return []byte("\x1b[B")
	case hal.KeyRight:
		return []byte("\x1b[C")
	case hal.KeyLeft:
		return []byte("\x1b[D")
	default:
		return nil
	}
}
