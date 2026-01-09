package termkbd

import (
	"spark/hal"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

type Service struct {
	in      hal.Input
	outCap  kernel.Capability
	outKind proto.Kind

	events  <-chan hal.KeyEvent
	pending []byte

	heldCode hal.KeyCode
	heldData []byte

	nextRepeatTick uint64
}

// New writes VT100 bytes directly to the terminal service.
func New(in hal.Input, termCap kernel.Capability) *Service {
	return &Service{in: in, outCap: termCap, outKind: proto.MsgTermWrite}
}

// NewInput writes VT100 bytes as input messages to an intermediate consumer (e.g. shell).
func NewInput(in hal.Input, inputCap kernel.Capability) *Service {
	return &Service{in: in, outCap: inputCap, outKind: proto.MsgTermInput}
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

	done := make(chan struct{})
	defer close(done)

	tickCh := make(chan uint64, 16)
	go func() {
		last := ctx.NowTick()
		for {
			select {
			case <-done:
				return
			default:
			}
			last = ctx.WaitTick(last)
			select {
			case tickCh <- last:
			default:
			}
		}
	}()

	for {
		select {
		case ev, ok := <-s.events:
			if !ok {
				return
			}
			s.handleKeyEvent(ctx, ev)
		case tick := <-tickCh:
			s.handleRepeat(tick)
			s.flush(ctx)
		}
	}
}

func (s *Service) handleKeyEvent(ctx *kernel.Context, ev hal.KeyEvent) {
	if !ev.Press {
		if s.heldData != nil && ev.Code == s.heldCode {
			s.heldData = nil
			s.nextRepeatTick = 0
		}
		return
	}

	data := vt100FromKey(ev)
	if len(data) > 0 {
		s.pending = append(s.pending, data...)
		s.flush(ctx)
	}

	if !repeatableKey(ev, data) {
		return
	}
	s.heldCode = ev.Code
	s.heldData = append(s.heldData[:0], data...)

	now := ctx.NowTick()
	s.nextRepeatTick = now + repeatDelayTicks
}

func (s *Service) handleRepeat(tick uint64) {
	if s.heldData == nil {
		return
	}
	if tick < s.nextRepeatTick {
		return
	}
	s.pending = append(s.pending, s.heldData...)
	s.nextRepeatTick = tick + repeatRateTicks
}

func (s *Service) flush(ctx *kernel.Context) {
	if len(s.pending) == 0 {
		return
	}
	if !s.outCap.Valid() {
		s.pending = nil
		return
	}

	chunk := s.pending
	if len(chunk) > kernel.MaxMessageBytes {
		chunk = chunk[:kernel.MaxMessageBytes]
	}

	res := ctx.SendToCapResult(s.outCap, uint16(s.outKind), chunk, kernel.Capability{})
	switch res {
	case kernel.SendOK:
		s.pending = s.pending[len(chunk):]
	case kernel.SendErrQueueFull:
	default:
		s.pending = nil
	}
}

const (
	// Ticks are 1ms on host and TinyGo.
	// These values aim to match typical desktop key-repeat feel without spamming.
	repeatDelayTicks = 350
	repeatRateTicks  = 60
)

func repeatableKey(ev hal.KeyEvent, data []byte) bool {
	if len(data) == 0 {
		return false
	}
	switch ev.Code {
	case hal.KeyUp, hal.KeyDown, hal.KeyLeft, hal.KeyRight,
		hal.KeyBackspace, hal.KeyDelete, hal.KeyHome, hal.KeyEnd:
		return true
	default:
		return false
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
	case hal.KeyBackspace:
		return []byte{0x7f}
	case hal.KeyTab:
		return []byte{'\t'}
	case hal.KeyUp:
		return []byte("\x1b[A")
	case hal.KeyDown:
		return []byte("\x1b[B")
	case hal.KeyRight:
		return []byte("\x1b[C")
	case hal.KeyLeft:
		return []byte("\x1b[D")
	case hal.KeyDelete:
		return []byte("\x1b[3~")
	case hal.KeyHome:
		return []byte("\x1b[H")
	case hal.KeyEnd:
		return []byte("\x1b[F")
	case hal.KeyF1:
		return []byte("\x1b[11~")
	case hal.KeyF2:
		return []byte("\x1b[12~")
	case hal.KeyF3:
		return []byte("\x1b[13~")
	default:
		return nil
	}
}
