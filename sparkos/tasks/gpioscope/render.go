package gpioscope

import (
	"fmt"
	"image/color"
	"strings"

	"spark/hal"
	"spark/sparkos/fonts/font6x8cp1251"

	"tinygo.org/x/drivers"
	"tinygo.org/x/tinyfont"
)

var (
	colorBG       = color.RGBA{R: 0x00, G: 0x00, B: 0x00, A: 0xff}
	colorFG       = color.RGBA{R: 0xee, G: 0xee, B: 0xee, A: 0xff}
	colorDim      = color.RGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xff}
	colorHeaderBG = color.RGBA{R: 0x18, G: 0x18, B: 0x18, A: 0xff}
	colorPanelBG  = color.RGBA{R: 0x08, G: 0x08, B: 0x08, A: 0xff}
	colorSelBG    = color.RGBA{R: 0xee, G: 0xee, B: 0xee, A: 0xff}
	colorSelFG    = color.RGBA{R: 0x11, G: 0x11, B: 0x11, A: 0xff}

	colorWaveHi = color.RGBA{R: 0x4a, G: 0xdf, B: 0x6a, A: 0xff}
	colorWaveLo = color.RGBA{R: 0x24, G: 0x24, B: 0x24, A: 0xff}
	colorCursor = color.RGBA{R: 0xff, G: 0xdd, B: 0x66, A: 0xff}
)

type fbDisplay struct {
	fb hal.Framebuffer
}

func newFBDisplay(fb hal.Framebuffer) *fbDisplay {
	return &fbDisplay{fb: fb}
}

func (d *fbDisplay) Size() (x, y int16) {
	if d.fb == nil {
		return 0, 0
	}
	return int16(d.fb.Width()), int16(d.fb.Height())
}

func (d *fbDisplay) SetPixel(x, y int16, c color.RGBA) {
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

	pixel := rgb565From888(c.R, c.G, c.B)
	off := iy*d.fb.StrideBytes() + ix*2
	if off < 0 || off+1 >= len(buf) {
		return
	}
	buf[off] = byte(pixel)
	buf[off+1] = byte(pixel >> 8)
}

func (d *fbDisplay) Display() error {
	if d.fb == nil {
		return nil
	}
	return d.fb.Present()
}

func (d *fbDisplay) FillRectangle(x, y, width, height int16, c color.RGBA) error {
	if d.fb == nil || d.fb.Format() != hal.PixelFormatRGB565 {
		return nil
	}
	buf := d.fb.Buffer()
	if buf == nil {
		return nil
	}

	w := d.fb.Width()
	h := d.fb.Height()

	x0 := clampInt(int(x), 0, w)
	y0 := clampInt(int(y), 0, h)
	x1 := clampInt(int(x)+int(width), 0, w)
	y1 := clampInt(int(y)+int(height), 0, h)
	if x0 >= x1 || y0 >= y1 {
		return nil
	}

	pixel := rgb565From888(c.R, c.G, c.B)
	lo := byte(pixel)
	hi := byte(pixel >> 8)

	stride := d.fb.StrideBytes()
	for py := y0; py < y1; py++ {
		row := py * stride
		for px := x0; px < x1; px++ {
			off := row + px*2
			if off < 0 || off+1 >= len(buf) {
				continue
			}
			buf[off] = lo
			buf[off+1] = hi
		}
	}
	return nil
}

func (d *fbDisplay) SetRotation(rotation drivers.Rotation) error {
	_ = rotation
	return nil
}

func rgb565From888(r, g, b uint8) uint16 {
	return uint16((uint16(r>>3)&0x1F)<<11 | (uint16(g>>2)&0x3F)<<5 | (uint16(b>>3) & 0x1F))
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func initFont() (font tinyfont.Fonter, fontWidth, fontHeight, fontOffset int16, ok bool) {
	font = font6x8cp1251.Font
	fontHeight = 8
	fontOffset = 7
	_, outboxWidth := tinyfont.LineWidth(font, "0")
	fontWidth = int16(outboxWidth)
	if fontWidth <= 0 || fontHeight <= 0 {
		return nil, 0, 0, 0, false
	}
	return font, fontWidth, fontHeight, fontOffset, true
}

func writeText(d *fbDisplay, font tinyfont.Fonter, x, y int16, c color.RGBA, s string) {
	tinyfont.WriteLine(d, font, x, y, s, c)
}

func fitText(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(s) <= max {
		return s
	}
	return s[:max]
}

func fmtHz(periodTicks uint32) string {
	if periodTicks == 0 {
		return "0Hz"
	}
	return fmt.Sprintf("%dHz", 1000/int(periodTicks))
}

func joinTrimLines(lines []string, max int) []string {
	if len(lines) <= max {
		return lines
	}
	return append([]string(nil), lines[len(lines)-max:]...)
}

func wrapLine(s string, width int) []string {
	if width <= 0 || s == "" {
		return nil
	}
	var out []string
	for len(s) > width {
		out = append(out, strings.TrimRight(s[:width], " "))
		s = strings.TrimLeft(s[width:], " ")
	}
	if s != "" {
		out = append(out, s)
	}
	return out
}
