package hexedit

import (
	"fmt"
	"image/color"
	"strings"

	"spark/hal"
	"spark/sparkos/fonts/font6x8cp1251"
	"spark/sparkos/kernel"

	"tinygo.org/x/drivers"
	"tinygo.org/x/tinyfont"
)

var (
	colorBG       = color.RGBA{R: 0, G: 0, B: 0, A: 0xff}
	colorFG       = color.RGBA{R: 0xee, G: 0xee, B: 0xee, A: 0xff}
	colorDim      = color.RGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xff}
	colorHeaderBG = color.RGBA{R: 0x22, G: 0x22, B: 0x22, A: 0xff}
	colorStatusBG = color.RGBA{R: 0x22, G: 0x22, B: 0x22, A: 0xff}
	colorPanelBG  = color.RGBA{R: 0x08, G: 0x08, B: 0x08, A: 0xff}
	colorSelBG    = color.RGBA{R: 0xee, G: 0xee, B: 0xee, A: 0xff}
	colorSelFG    = color.RGBA{R: 0x11, G: 0x11, B: 0x11, A: 0xff}
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
	t.font = font6x8cp1251.Font
	t.fontHeight = 8
	t.fontOffset = 7
	_, outboxWidth := tinyfont.LineWidth(t.font, "0")
	t.fontWidth = int16(outboxWidth)
	return t.fontWidth > 0 && t.fontHeight > 0
}

func (t *Task) render(ctx *kernel.Context) {
	if !t.active || t.fb == nil || t.d == nil {
		return
	}
	w := int16(t.fb.Width())
	h := int16(t.fb.Height())
	if w <= 0 || h <= 0 {
		return
	}

	_ = t.d.FillRectangle(0, 0, w, h, colorBG)

	headerY := int16(0)
	_ = t.d.FillRectangle(0, headerY, w, t.fontHeight, colorHeaderBG)
	t.drawStringClipped(0, headerY, t.headerText(), colorFG, t.cols)

	panelY := t.fontHeight
	viewH := int16(t.viewRows) * t.fontHeight
	_ = t.d.FillRectangle(0, panelY, w, viewH, colorPanelBG)

	if t.path == "" {
		t.drawStringClipped(0, panelY, "hex: select a file (e.g. `hex /path/to/file`)", colorDim, t.cols)
		t.renderStatus(w)
		if t.showHelp {
			t.renderHelp()
		}
		_ = t.fb.Present()
		return
	}

	if t.viewASCII {
		t.renderASCII(ctx, panelY)
		t.renderStatus(w)
		if t.showHelp {
			t.renderHelp()
		}
		_ = t.fb.Present()
		return
	}

	bytesPerRow, showASCII := t.layout()
	t.ensureVisible(bytesPerRow)

	hexStartCol := 10
	asciiStartCol := hexStartCol + bytesPerRow*3 + 1

	for row := 0; row < t.viewRows; row++ {
		lineOff := uint32(t.topRow+row) * uint32(bytesPerRow)
		if t.size != 0 && lineOff >= t.size {
			break
		}
		y := panelY + int16(row)*t.fontHeight

		t.drawHexOffset(0, y, lineOff)
		t.drawCell(int16(8)*t.fontWidth, y, ':', colorDim)
		t.drawCell(int16(9)*t.fontWidth, y, ' ', colorDim)

		for i := 0; i < bytesPerRow; i++ {
			off := lineOff + uint32(i)
			col := hexStartCol + i*3
			x := int16(col) * t.fontWidth

			b, ok := t.byteAt(ctx, off)
			if !ok {
				t.drawCell(x, y, ' ', colorDim)
				t.drawCell(x+t.fontWidth, y, ' ', colorDim)
				if showASCII {
					t.drawCell(int16(asciiStartCol+i)*t.fontWidth, y, ' ', colorDim)
				}
				continue
			}

			sel := off == t.cursor
			fg := colorFG
			if sel {
				_ = t.d.FillRectangle(x, y, 2*t.fontWidth, t.fontHeight, colorSelBG)
				fg = colorSelFG
			}
			t.drawHexByte(x, y, b, fg)

			if showASCII {
				ax := int16(asciiStartCol+i) * t.fontWidth
				afg := colorFG
				if sel {
					_ = t.d.FillRectangle(ax, y, t.fontWidth, t.fontHeight, colorSelBG)
					afg = colorSelFG
				}
				t.drawCell(ax, y, printableASCII(b), afg)
			}
		}
	}

	t.renderStatus(w)
	if t.showHelp {
		t.renderHelp()
	}
	_ = t.fb.Present()
}

func (t *Task) renderASCII(ctx *kernel.Context, panelY int16) {
	bytesPerRow := t.layoutASCIIBytesPerRow()
	t.ensureVisible(bytesPerRow)

	asciiStartCol := 10
	for row := 0; row < t.viewRows; row++ {
		lineOff := uint32(t.topRow+row) * uint32(bytesPerRow)
		if t.size != 0 && lineOff >= t.size {
			break
		}
		y := panelY + int16(row)*t.fontHeight

		t.drawHexOffset(0, y, lineOff)
		t.drawCell(int16(8)*t.fontWidth, y, ':', colorDim)
		t.drawCell(int16(9)*t.fontWidth, y, ' ', colorDim)

		for i := 0; i < bytesPerRow; i++ {
			off := lineOff + uint32(i)
			b, ok := t.byteAt(ctx, off)
			if !ok {
				continue
			}
			ax := int16(asciiStartCol+i) * t.fontWidth
			sel := off == t.cursor
			fg := colorFG
			if sel {
				_ = t.d.FillRectangle(ax, y, t.fontWidth, t.fontHeight, colorSelBG)
				fg = colorSelFG
			}
			t.drawCell(ax, y, printableASCII(b), fg)
		}
	}
}

func (t *Task) renderStatus(w int16) {
	statusY := int16(t.rows-1) * t.fontHeight
	_ = t.d.FillRectangle(0, statusY, w, t.fontHeight, colorStatusBG)
	t.drawStringClipped(0, statusY, t.statusText(), colorFG, t.cols)
}

func (t *Task) renderHelp() {
	lines := []string{
		"hexedit help",
		"",
		"Movement",
		"  arrows/home/end: move cursor",
		"  PgUp/PgDn: page",
		"",
		"Edit",
		"  v: toggle view (HEX/ASCII)",
		"  i: toggle HEX/ASCII input",
		"  0-9 a-f: edit byte (HEX)",
		"  printable chars: edit byte (ASCII)",
		"  w: save (rewrite file)",
		"",
		"Search / goto",
		"  g: goto offset",
		"  /: find ASCII, ?: find HEX bytes",
		"  n/N: next/prev match",
		"",
		"Other",
		"  H: toggle this help",
		"  q/ESC: exit (asks if dirty)",
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
	contentRows := boxRows - 3
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

	title := "hexedit  (H/Esc close, Up/Down scroll)"
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
		if lines[i] == "" || strings.HasSuffix(lines[i], "help") {
			fg = colorDim
		}
		t.drawStringClipped(px+t.fontWidth, y, lines[i], fg, innerCols)
	}
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

func (t *Task) drawCell(x, y int16, r rune, fg color.RGBA) {
	tinyfont.DrawChar(t.d, t.font, x, y+t.fontOffset, r, fg)
}

func (t *Task) drawHexOffset(x, y int16, off uint32) {
	const digits = "0123456789ABCDEF"
	for i := 0; i < 8; i++ {
		shift := uint((7 - i) * 4)
		n := byte((off >> shift) & 0x0f)
		t.drawCell(x+int16(i)*t.fontWidth, y, rune(digits[n]), colorDim)
	}
}

func (t *Task) drawHexByte(x, y int16, b byte, fg color.RGBA) {
	const digits = "0123456789ABCDEF"
	hi := digits[(b>>4)&0x0f]
	lo := digits[b&0x0f]
	t.drawCell(x, y, rune(hi), fg)
	t.drawCell(x+t.fontWidth, y, rune(lo), fg)
}

func printableASCII(b byte) rune {
	if b >= 0x20 && b <= 0x7e {
		return rune(b)
	}
	return '.'
}

func (t *Task) headerText() string {
	if t.path == "" {
		return "HEXEDIT"
	}
	dirty := ""
	if t.dirty {
		dirty = " *"
	}
	return fmt.Sprintf("HEXEDIT %s%s", t.path, dirty)
}
