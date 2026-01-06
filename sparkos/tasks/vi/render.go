//go:build spark_vi

package vi

import (
	"fmt"
	"image/color"

	"spark/hal"

	"tinygo.org/x/drivers"
	"tinygo.org/x/tinyfont"
)

var (
	colorBG     = color.RGBA{R: 0, G: 0, B: 0, A: 0xff}
	colorFG     = color.RGBA{R: 0xee, G: 0xee, B: 0xee, A: 0xff}
	colorTilde  = color.RGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xff}
	colorStatus = color.RGBA{R: 0x22, G: 0x22, B: 0x22, A: 0xff}
	colorCursor = color.RGBA{R: 0xee, G: 0xee, B: 0xee, A: 0xff}
	colorInvFG  = color.RGBA{R: 0x11, G: 0x11, B: 0x11, A: 0xff}
)

func (t *Task) render() {
	if !t.active || t.fb == nil || t.d == nil {
		return
	}

	w := int16(t.fb.Width())
	h := int16(t.fb.Height())
	if w <= 0 || h <= 0 {
		return
	}

	_ = t.d.FillRectangle(0, 0, w, h, colorBG)

	for row := 0; row < t.viewRows; row++ {
		y := int16(row) * t.fontHeight
		lineIdx := t.editor.topLine + row
		if lineIdx < 0 || lineIdx >= len(t.editor.lines) {
			t.drawCell(0, y, '~', colorTilde)
			continue
		}

		line := t.editor.lines[lineIdx]
		start := t.editor.leftCol
		if start < 0 {
			start = 0
		}
		if start >= len(line) {
			continue
		}
		vis := line[start:]
		if len(vis) > t.cols {
			vis = vis[:t.cols]
		}
		t.drawRunes(0, y, vis, colorFG)
	}

	statusY := int16(t.viewRows) * t.fontHeight
	_ = t.d.FillRectangle(0, statusY, w, t.fontHeight, colorStatus)
	status := t.statusText()
	t.drawString(0, statusY, status, colorFG)

	t.drawCursor(statusY)
	_ = t.fb.Present()
}

func (t *Task) statusText() string {
	switch t.editor.mode {
	case modeCmdline:
		return ":" + string(t.editor.cmdline)
	case modeSearch:
		return "/" + string(t.editor.search)
	}

	mode := "NORMAL"
	if t.editor.mode == modeInsert {
		mode = "INSERT"
	}
	name := t.editor.filePath
	if name == "" {
		name = "[No Name]"
	}
	mod := ""
	if t.editor.modified {
		mod = " [+]"
	}
	pos := fmt.Sprintf("%d:%d", t.editor.cursorLine+1, t.editor.cursorCol+1)

	out := mode + " " + name + mod + " " + pos
	if t.editor.message != "" {
		out += " | " + t.editor.message
	}

	max := t.cols
	rs := []rune(out)
	if len(rs) > max {
		rs = rs[:max]
	}
	return string(rs)
}

func (t *Task) drawCursor(statusY int16) {
	row := -1
	col := -1
	switch t.editor.mode {
	case modeCmdline:
		row = t.viewRows
		col = 1 + t.editor.cmdPos
	case modeSearch:
		row = t.viewRows
		col = 1 + t.editor.searchPos
	default:
		row = t.editor.cursorLine - t.editor.topLine
		col = t.editor.cursorCol - t.editor.leftCol
	}

	if row < 0 || row > t.viewRows {
		return
	}
	if col < 0 || col >= t.cols {
		return
	}

	var r rune
	switch t.editor.mode {
	case modeCmdline:
		if t.editor.cmdPos >= 0 && t.editor.cmdPos < len(t.editor.cmdline) {
			r = t.editor.cmdline[t.editor.cmdPos]
		} else {
			r = ' '
		}
	case modeSearch:
		if t.editor.searchPos >= 0 && t.editor.searchPos < len(t.editor.search) {
			r = t.editor.search[t.editor.searchPos]
		} else {
			r = ' '
		}
	default:
		if t.editor.cursorLine < 0 || t.editor.cursorLine >= len(t.editor.lines) {
			return
		}
		line := t.editor.lines[t.editor.cursorLine]
		if t.editor.cursorCol >= 0 && t.editor.cursorCol < len(line) {
			r = line[t.editor.cursorCol]
		} else {
			r = ' '
		}
	}

	x := int16(col) * t.fontWidth
	y := int16(row) * t.fontHeight
	if row == t.viewRows {
		y = statusY
	}

	_ = t.d.FillRectangle(x, y, t.fontWidth, t.fontHeight, colorCursor)
	t.drawCell(x, y, r, colorInvFG)
}

func (t *Task) drawString(x, y int16, s string, fg color.RGBA) {
	t.drawRunes(x, y, []rune(s), fg)
}

func (t *Task) drawRunes(x, y int16, rs []rune, fg color.RGBA) {
	col := int16(0)
	for _, r := range rs {
		if int(col) >= t.cols {
			return
		}
		tinyfont.DrawChar(t.d, t.font, x+col*t.fontWidth, y+t.fontOffset, r, fg)
		col++
	}
}

func (t *Task) drawCell(x, y int16, r rune, fg color.RGBA) {
	tinyfont.DrawChar(t.d, t.font, x, y+t.fontOffset, r, fg)
}

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
