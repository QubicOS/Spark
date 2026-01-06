package ui

import (
	"spark/hal"
	"spark/sparkos/kernel"
)

type Service struct {
	disp hal.Display
	in   hal.Input

	fb     hal.Framebuffer
	events <-chan hal.KeyEvent

	x int
	y int
}

func New(d hal.Display, in hal.Input) *Service {
	return &Service{disp: d, in: in}
}

func (s *Service) Step(ctx *kernel.Context) {
	_ = ctx

	if s.fb == nil && s.disp != nil {
		s.fb = s.disp.Framebuffer()
		if s.fb != nil {
			s.x = s.fb.Width() / 2
			s.y = s.fb.Height() / 2
			s.fb.ClearRGB(0, 0, 0)
			_ = s.fb.Present()
		}
	}

	if s.events == nil && s.in != nil {
		if kbd := s.in.Keyboard(); kbd != nil {
			s.events = kbd.Events()
		}
	}

	if s.fb == nil || s.events == nil {
		return
	}

	changed := false
	for {
		var ev hal.KeyEvent
		select {
		case ev = <-s.events:
		default:
			if changed {
				drawCursorRGB565(s.fb, s.x, s.y, 255, 255, 255)
				_ = s.fb.Present()
			}
			return
		}

		switch ev.Code {
		case hal.KeyUp:
			if s.y > 0 {
				s.y--
				changed = true
			}
		case hal.KeyDown:
			if s.y < s.fb.Height()-1 {
				s.y++
				changed = true
			}
		case hal.KeyLeft:
			if s.x > 0 {
				s.x--
				changed = true
			}
		case hal.KeyRight:
			if s.x < s.fb.Width()-1 {
				s.x++
				changed = true
			}
		case hal.KeyEnter:
			s.fb.ClearRGB(0, 0, 0)
			changed = true
		}
	}
}

func drawCursorRGB565(fb hal.Framebuffer, x, y int, r, g, b uint8) {
	if fb.Format() != hal.PixelFormatRGB565 {
		return
	}
	buf := fb.Buffer()
	if buf == nil {
		return
	}

	w := fb.Width()
	h := fb.Height()
	if x < 0 || x >= w || y < 0 || y >= h {
		return
	}

	pixel := uint16((uint16(r>>3)&0x1F)<<11 | (uint16(g>>2)&0x3F)<<5 | (uint16(b>>3) & 0x1F))
	off := y*fb.StrideBytes() + x*2
	if off < 0 || off+1 >= len(buf) {
		return
	}
	buf[off] = byte(pixel)
	buf[off+1] = byte(pixel >> 8)
}

