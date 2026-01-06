package app

import (
	"fmt"
	"image/color"
	"strings"
	"unicode/utf8"

	"spark/hal"
	"spark/sparkos/fonts/const2bitcolor"
	"spark/sparkos/fonts/dejavumono9"
	"spark/sparkos/kernel"

	"tinygo.org/x/tinyfont"
)

func installPanicHandler(h hal.HAL) {
	kernel.SetPanicHandler(func(info kernel.PanicInfo) {
		if l := h.Logger(); l != nil {
			l.WriteLineString(fmt.Sprintf("Spark Panic: task=%d panic=%v", info.TaskID, info.Value))
			if len(info.Stack) > 0 {
				for _, line := range strings.Split(string(info.Stack), "\n") {
					if line == "" {
						continue
					}
					l.WriteLineString(line)
				}
			}
		}

		disp := h.Display()
		if disp == nil {
			select {}
		}
		fb := disp.Framebuffer()
		if fb == nil {
			select {}
		}

		fb.ClearRGB(255, 255, 255)

		font := &dejavumono9.DejaVuSansMono9
		fontHeight, fontOffset := int16(11), int16(8)
		if f, ok := any(font).(*const2bitcolor.Font); ok {
			if h, off, err := const2bitcolor.ComputeTerminalMetrics(f); err == nil {
				fontHeight = h
				fontOffset = off
			}
		}
		_, outboxWidth := tinyfont.LineWidth(font, "0")
		fontWidth := int16(outboxWidth)
		if fontWidth <= 0 || fontHeight <= 0 {
			_ = fb.Present()
			select {}
		}

		d := panicDisplay{fb: fb}

		lines := []string{
			"Spark Panic:",
			fmt.Sprintf("task: %d", info.TaskID),
			fmt.Sprintf("panic: %v", info.Value),
		}
		if len(info.Stack) > 0 {
			lines = append(lines, "stack:")
			for _, line := range strings.Split(string(info.Stack), "\n") {
				if line == "" {
					continue
				}
				lines = append(lines, line)
			}
		} else {
			lines = append(lines, "stack: unavailable")
		}

		fg := color.RGBA{R: 0, G: 0, B: 0, A: 255}

		y := int16(0)
		maxW, maxH := fb.Width(), fb.Height()
		cols := int16(maxW) / fontWidth
		if cols <= 0 {
			cols = 1
		}

		for _, line := range lines {
			for len(line) > 0 {
				if y+fontHeight > int16(maxH) {
					_ = fb.Present()
					select {}
				}
				chunk, rest := takeRunes(line, cols)
				drawTextLine(d, font, fontWidth, fontOffset, 0, y, chunk, fg)
				y += fontHeight
				line = strings.TrimLeft(rest, " ")
			}
			if y+fontHeight > int16(maxH) {
				break
			}
		}

		_ = fb.Present()
		select {}
	})
}

func drawTextLine(
	d panicDisplay,
	font tinyfont.Fonter,
	fontWidth, fontOffset int16,
	x0, y0 int16,
	s string,
	fg color.RGBA,
) {
	var drawX = x0
	for _, r := range s {
		tinyfont.DrawChar(d, font, drawX, y0+fontOffset, r, fg)
		drawX += fontWidth
	}
}

type panicDisplay struct {
	fb hal.Framebuffer
}

func (d panicDisplay) Size() (x, y int16) {
	if d.fb == nil {
		return 0, 0
	}
	return int16(d.fb.Width()), int16(d.fb.Height())
}

func (d panicDisplay) SetPixel(x, y int16, c color.RGBA) {
	if d.fb == nil || d.fb.Format() != hal.PixelFormatRGB565 {
		return
	}
	buf := d.fb.Buffer()
	if buf == nil {
		return
	}

	w := d.fb.Width()
	h := d.fb.Height()
	ix := int(x)
	iy := int(y)
	if ix < 0 || ix >= w || iy < 0 || iy >= h {
		return
	}

	pixel := uint16((uint16(c.R>>3)&0x1F)<<11 | (uint16(c.G>>2)&0x3F)<<5 | (uint16(c.B>>3) & 0x1F))
	off := iy*d.fb.StrideBytes() + ix*2
	if off < 0 || off+1 >= len(buf) {
		return
	}
	buf[off] = byte(pixel)
	buf[off+1] = byte(pixel >> 8)
}

func (d panicDisplay) Display() error { return nil }

func takeRunes(s string, n int16) (prefix, rest string) {
	if n <= 0 || s == "" {
		return "", s
	}
	if int64(len(s)) <= int64(n) {
		return s, ""
	}
	var i int
	var count int16
	for i < len(s) && count < n {
		_, size := utf8.DecodeRuneInString(s[i:])
		if size <= 0 {
			break
		}
		i += size
		count++
	}
	if i >= len(s) {
		return s, ""
	}
	return s[:i], s[i:]
}
