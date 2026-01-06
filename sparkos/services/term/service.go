package term

import (
	"errors"
	"fmt"

	"spark/hal"
	"spark/sparkos/fonts/const2bitcolor"
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
			case proto.MsgTermRefresh:
				s.t.Display()
				dirty = false
			}
		}
	}
}

func (s *Service) reset() {
	s.t = tinyterm.NewTerminal(s.d)

	font := &dejavumono9.DejaVuSansMono9
	fontHeight, fontOffset, err := computeFontMetrics(font)
	if err != nil {
		fontHeight = 11
		fontOffset = 8
	}

	s.t.Configure(&tinyterm.Config{
		Font:              font,
		FontHeight:        fontHeight,
		FontOffset:        fontOffset,
		UseSoftwareScroll: true,
	})
	s.fb.ClearRGB(0, 0, 0)
	_ = s.fb.Present()
}

func computeFontMetrics(font *const2bitcolor.Font) (fontHeight int16, fontOffset int16, err error) {
	if font == nil {
		return 0, 0, errors.New("nil font")
	}
	if len(font.OffsetMap)%6 != 0 {
		return 0, 0, errors.New("bad font offset map")
	}
	if len(font.Data) < 5 {
		return 0, 0, errors.New("bad font data")
	}

	entries := len(font.OffsetMap) / 6
	minY := 0
	maxY := 0
	first := true

	for i := 0; i < entries; i++ {
		base := i * 6
		off := int(uint32(font.OffsetMap[base+3])<<16 | uint32(font.OffsetMap[base+4])<<8 | uint32(font.OffsetMap[base+5]))
		if off < 0 || off+5 >= len(font.Data) {
			continue
		}

		h := int(font.Data[off+1])
		yoff := int(int8(font.Data[off+4]))
		if first {
			minY = yoff
			maxY = yoff + h
			first = false
			continue
		}
		if yoff < minY {
			minY = yoff
		}
		if yoff+h > maxY {
			maxY = yoff + h
		}
	}
	if first {
		return 0, 0, errors.New("no glyphs")
	}

	height := maxY - minY
	offset := -minY
	if height <= 0 || offset < 0 {
		return 0, 0, fmt.Errorf("invalid font metrics: height=%d offset=%d", height, offset)
	}
	if height > 127 || offset > 127 {
		return 0, 0, fmt.Errorf("font metrics too large: height=%d offset=%d", height, offset)
	}
	return int16(height), int16(offset), nil
}
