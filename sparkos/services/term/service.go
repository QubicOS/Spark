package term

import (
	"spark/hal"
	"spark/sparkos/fonts/font6x8cp1251"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"

	"tinygo.org/x/tinyterm"
)

type Service struct {
	disp hal.Display
	ep   kernel.Capability

	fb hal.Framebuffer
	d  *fbDisplay
	t  *tinyterm.Terminal

	saved      []byte
	savedValid bool
}

func New(disp hal.Display, ep kernel.Capability) *Service {
	return &Service{disp: disp, ep: ep}
}

func (s *Service) Run(ctx *kernel.Context) {
	ch, ok := ctx.RecvChan(s.ep)
	if !ok {
		return
	}

	if s.disp == nil {
		return
	}
	s.fb = s.disp.Framebuffer()
	if s.fb == nil {
		return
	}

	s.d = newFBDisplay(s.fb)
	s.reset()

	dirty := false

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
		case <-tickCh:
			if dirty {
				s.t.Display()
				dirty = false
			}

		case msg, ok := <-ch:
			if !ok {
				return
			}
			switch proto.Kind(msg.Kind) {
			case proto.MsgTermWrite:
				_, _ = s.t.Write(msg.Payload())
				dirty = true
			case proto.MsgTermClear:
				s.reset()
				dirty = true
			case proto.MsgTermRefresh:
				s.t.Display()
				dirty = false
			case proto.MsgTermScreenSave:
				s.saveScreen()
			case proto.MsgTermScreenRestore:
				s.restoreScreen()
			}
		}
	}
}

func (s *Service) reset() {
	s.t = tinyterm.NewTerminal(s.d)

	s.t.Configure(&tinyterm.Config{
		Font:              font6x8cp1251.Font,
		FontHeight:        8,
		FontOffset:        7,
		UseSoftwareScroll: true,
	})
	s.fb.ClearRGB(0, 0, 0)
	_ = s.fb.Present()
}

func (s *Service) saveScreen() {
	if s.fb == nil {
		return
	}
	buf := s.fb.Buffer()
	if buf == nil {
		return
	}
	size := s.fb.StrideBytes() * s.fb.Height()
	if size <= 0 {
		return
	}
	if size > len(buf) {
		size = len(buf)
	}
	if cap(s.saved) < size {
		s.saved = make([]byte, size)
	} else {
		s.saved = s.saved[:size]
	}
	copy(s.saved, buf[:size])
	s.savedValid = true
}

func (s *Service) restoreScreen() {
	if !s.savedValid || s.fb == nil {
		return
	}
	buf := s.fb.Buffer()
	if buf == nil {
		return
	}
	size := s.fb.StrideBytes() * s.fb.Height()
	if size <= 0 {
		return
	}
	if size > len(buf) {
		size = len(buf)
	}
	if size > len(s.saved) {
		size = len(s.saved)
	}
	copy(buf[:size], s.saved[:size])
	_ = s.fb.Present()
}
