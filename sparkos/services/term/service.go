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
				// Display immediately to avoid relying on tick scheduling.
				// This keeps the shell responsive during early boot on baremetal.
				s.t.Display()
				dirty = false
			case proto.MsgTermClear:
				s.reset()
				s.t.Display()
				dirty = false
			case proto.MsgTermRefresh:
				s.t.Display()
				dirty = false
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
	_, _ = s.t.Write([]byte("term: ready\n"))
	_ = s.fb.Present()
}
