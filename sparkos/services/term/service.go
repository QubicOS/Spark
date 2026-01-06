package term

import (
	"spark/hal"
	"spark/sparkos/fonts/dejavumono9"
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
		case <-tickCh:
			if dirty {
				s.t.Display()
				dirty = false
			}

		case msg := <-ch:
			switch proto.Kind(msg.Kind) {
			case proto.MsgTermWrite:
				_, _ = s.t.Write(msg.Data[:msg.Len])
				dirty = true
			case proto.MsgTermClear:
				s.reset()
				dirty = true
			}
		}
	}
}

func (s *Service) reset() {
	s.t = tinyterm.NewTerminal(s.d)
	s.t.Configure(&tinyterm.Config{
		Font:              &dejavumono9.DejaVuSansMono9,
		FontHeight:        11,
		FontOffset:        8,
		UseSoftwareScroll: true,
	})
	s.fb.ClearRGB(0, 0, 0)
	_ = s.fb.Present()
}
