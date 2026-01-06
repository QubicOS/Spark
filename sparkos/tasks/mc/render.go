package mc

import (
	"fmt"
	"image/color"

	"spark/hal"
	"spark/sparkos/fonts/const2bitcolor"
	"spark/sparkos/fonts/dejavumono9"

	"tinygo.org/x/drivers"
	"tinygo.org/x/tinyfont"
)

var (
	colorBG        = color.RGBA{R: 0, G: 0, B: 0, A: 0xff}
	colorFG        = color.RGBA{R: 0xee, G: 0xee, B: 0xee, A: 0xff}
	colorDim       = color.RGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xff}
	colorHeaderBG  = color.RGBA{R: 0x22, G: 0x22, B: 0x22, A: 0xff}
	colorStatusBG  = color.RGBA{R: 0x22, G: 0x22, B: 0x22, A: 0xff}
	colorPanelBG   = color.RGBA{R: 0x08, G: 0x08, B: 0x08, A: 0xff}
	colorActiveBG  = color.RGBA{R: 0x10, G: 0x10, B: 0x10, A: 0xff}
	colorSelBG     = color.RGBA{R: 0xee, G: 0xee, B: 0xee, A: 0xff}
	colorSelFG     = color.RGBA{R: 0x11, G: 0x11, B: 0x11, A: 0xff}
	colorSep       = color.RGBA{R: 0x66, G: 0x66, B: 0x66, A: 0xff}
	colorDir       = color.RGBA{R: 0x9a, G: 0xb9, B: 0xff, A: 0xff}
	colorActiveDir = color.RGBA{R: 0x7f, G: 0xc9, B: 0xff, A: 0xff}
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

func (t *Task) initFont() bool {
	t.font = &dejavumono9.DejaVuSansMono9
	t.fontHeight, t.fontOffset = 11, 8
	if f, ok := t.font.(*const2bitcolor.Font); ok {
		if h, off, err := const2bitcolor.ComputeTerminalMetrics(f); err == nil {
			t.fontHeight = h
			t.fontOffset = off
		}
	}
	_, outboxWidth := tinyfont.LineWidth(t.font, "0")
	t.fontWidth = int16(outboxWidth)
	return t.fontWidth > 0 && t.fontHeight > 0
}

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

	if t.mode == modeViewer {
		t.renderViewer(w)
		if t.showHelp {
			t.renderHelp()
		}
		_ = t.fb.Present()
		return
	}

	headerY := int16(0)
	_ = t.d.FillRectangle(0, headerY, w, t.fontHeight, colorHeaderBG)
	t.drawString(0, headerY, t.headerText(), colorFG)

	panelTop := t.fontHeight
	viewH := int16(t.viewRows) * t.fontHeight
	_ = t.d.FillRectangle(0, panelTop, w, viewH, colorPanelBG)
	sepX := int16(t.panelCols) * t.fontWidth
	_ = t.d.FillRectangle(sepX, panelTop, t.fontWidth, viewH, colorBG)
	for y := int16(0); y < viewH; y += t.fontHeight {
		t.drawCell(sepX, panelTop+y, '|', colorSep)
	}

	t.renderPanel(0, &t.left, t.activePanel == panelLeft)
	t.renderPanel(t.panelCols+1, &t.right, t.activePanel == panelRight)

	statusY := int16(t.rows-1) * t.fontHeight
	_ = t.d.FillRectangle(0, statusY, w, t.fontHeight, colorStatusBG)
	t.drawString(0, statusY, t.statusText(), colorFG)

	if t.showHelp {
		t.renderHelp()
	}

	_ = t.fb.Present()
}

func (t *Task) renderPanel(x0 int, p *panel, active bool) {
	bg := colorPanelBG
	dirColor := colorDir
	if active {
		bg = colorActiveBG
		dirColor = colorActiveDir
	}

	panelX := int16(x0) * t.fontWidth
	panelY := t.fontHeight
	panelW := int16(t.panelCols) * t.fontWidth
	panelH := int16(t.viewRows) * t.fontHeight
	_ = t.d.FillRectangle(panelX, panelY, panelW, panelH, bg)

	p.clamp(t.viewRows)
	for row := 0; row < t.viewRows; row++ {
		idx := p.scroll + row
		y := panelY + int16(row)*t.fontHeight
		if idx < 0 || idx >= len(p.entries) {
			continue
		}
		e := p.entries[idx]
		fg := colorFG
		if e.isDir() {
			fg = dirColor
		}
		lineBG := bg
		if idx == p.sel && active {
			lineBG = colorSelBG
			fg = colorSelFG
		}
		_ = t.d.FillRectangle(panelX, y, panelW, t.fontHeight, lineBG)

		name := e.Name
		if e.isDir() && e.Name != ".." {
			name += "/"
		}
		t.drawStringClipped(panelX, y, name, fg, t.panelCols)

		if e.Name != ".." && !e.isDir() && e.Size > 0 {
			sz := fmt.Sprintf("%d", e.Size)
			t.drawRightAligned(panelX, y, sz, fg, t.panelCols)
		}
	}
}

func (t *Task) renderViewer(w int16) {
	headerY := int16(0)
	_ = t.d.FillRectangle(0, headerY, w, t.fontHeight, colorHeaderBG)
	t.drawString(0, headerY, t.viewerHeaderText(), colorFG)

	panelY := t.fontHeight
	viewH := int16(t.viewRows) * t.fontHeight
	_ = t.d.FillRectangle(0, panelY, w, viewH, colorPanelBG)

	for row := 0; row < t.viewRows; row++ {
		idx := t.viewerTop + row
		if idx < 0 || idx >= len(t.viewerLines) {
			continue
		}
		y := panelY + int16(row)*t.fontHeight
		t.drawRunesClipped(0, y, t.viewerLines[idx], colorFG, t.cols)
	}

	statusY := int16(t.rows-1) * t.fontHeight
	_ = t.d.FillRectangle(0, statusY, w, t.fontHeight, colorStatusBG)
	t.drawString(0, statusY, t.viewerStatusText(), colorFG)
}

func (t *Task) renderHelp() {
	lines := []string{
		"Panels",
		"TAB / LEFT RIGHT: switch panel",
		"UP DOWN: move",
		"HOME/END: top/bottom",
		"ENTER: open dir / view file",
		"BACKSPACE: parent dir",
		"c: copy file to other panel",
		"n: mkdir (auto name)",
		"r or Ctrl+R: refresh",
		"q or ESC: quit",
		"",
		"Viewer",
		"UP DOWN or j/k: scroll",
		"HOME/END: top/bottom",
		"q or ESC: back",
	}

	boxCols := t.cols - 4
	boxRows := t.rows - 4
	if boxCols < 20 || boxRows < 8 {
		return
	}

	maxLen := 0
	for _, ln := range lines {
		if n := len([]rune(ln)); n > maxLen {
			maxLen = n
		}
	}
	if maxLen+4 < boxCols {
		boxCols = maxLen + 4
	}
	if boxCols < 20 {
		boxCols = 20
	}
	if boxRows < 8 {
		boxRows = 8
	}

	innerCols := boxCols - 2
	contentRows := boxRows - 4
	if innerCols <= 0 || contentRows <= 0 {
		return
	}

	maxTop := len(lines) - contentRows
	if maxTop < 0 {
		maxTop = 0
	}
	if t.helpTop < 0 {
		t.helpTop = 0
	}
	if t.helpTop > maxTop {
		t.helpTop = maxTop
	}

	x0 := int16((t.cols - boxCols) / 2)
	y0 := int16((t.rows - boxRows) / 2)
	px := x0 * t.fontWidth
	py := y0 * t.fontHeight
	pw := int16(boxCols) * t.fontWidth
	ph := int16(boxRows) * t.fontHeight

	_ = t.d.FillRectangle(px, py, pw, ph, colorHeaderBG)
	_ = t.d.FillRectangle(px+t.fontWidth, py+t.fontHeight, pw-2*t.fontWidth, ph-2*t.fontHeight, colorPanelBG)

	title := "mc help  (H/Esc close, Up/Down scroll)"
	t.drawStringClipped(px+t.fontWidth, py+t.fontHeight, title, colorFG, innerCols)

	start := t.helpTop
	end := start + contentRows
	if end > len(lines) {
		end = len(lines)
	}
	for i := start; i < end; i++ {
		row := i - start
		y := py + int16(2+row)*t.fontHeight
		fg := colorFG
		if lines[i] == "" {
			fg = colorDim
		}
		t.drawStringClipped(px+t.fontWidth, y, lines[i], fg, innerCols)
	}
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

func (t *Task) drawStringClipped(x, y int16, s string, fg color.RGBA, cols int) {
	t.drawRunesClipped(x, y, []rune(s), fg, cols)
}

func (t *Task) drawRunesClipped(x, y int16, rs []rune, fg color.RGBA, cols int) {
	col := int16(0)
	for _, r := range rs {
		if int(col) >= cols {
			return
		}
		tinyfont.DrawChar(t.d, t.font, x+col*t.fontWidth, y+t.fontOffset, r, fg)
		col++
	}
}

func (t *Task) drawRightAligned(x, y int16, s string, fg color.RGBA, cols int) {
	rs := []rune(s)
	if len(rs) > cols {
		rs = rs[len(rs)-cols:]
	}
	start := cols - len(rs)
	if start < 0 {
		start = 0
	}
	t.drawRunesClipped(x+int16(start)*t.fontWidth, y, rs, fg, cols-start)
}
