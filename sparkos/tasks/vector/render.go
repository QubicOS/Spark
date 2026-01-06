package vector

import (
	"image/color"
	"math"

	"spark/hal"
	"spark/sparkos/fonts/font6x8cp1251"

	"tinygo.org/x/drivers"
	"tinygo.org/x/tinyfont"
)

var (
	colorBG       = color.RGBA{R: 0x00, G: 0x00, B: 0x00, A: 0xFF}
	colorFG       = color.RGBA{R: 0xEE, G: 0xEE, B: 0xEE, A: 0xFF}
	colorDim      = color.RGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xFF}
	colorHeaderBG = color.RGBA{R: 0x22, G: 0x22, B: 0x22, A: 0xFF}
	colorStatusBG = color.RGBA{R: 0x22, G: 0x22, B: 0x22, A: 0xFF}
	colorPanelBG  = color.RGBA{R: 0x08, G: 0x08, B: 0x08, A: 0xFF}
	colorGrid     = color.RGBA{R: 0x22, G: 0x22, B: 0x22, A: 0xFF}
	colorAxis     = color.RGBA{R: 0x55, G: 0x55, B: 0x55, A: 0xFF}
	colorPlot0    = color.RGBA{R: 0x4A, G: 0xD1, B: 0xFF, A: 0xFF}
	colorPlot1    = color.RGBA{R: 0xFF, G: 0xD1, B: 0x4A, A: 0xFF}
	colorPlot2    = color.RGBA{R: 0x7F, G: 0xFF, B: 0x7F, A: 0xFF}
	colorPlot3    = color.RGBA{R: 0xFF, G: 0x7F, B: 0xFF, A: 0xFF}
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

	headerY := int16(0)
	_ = t.d.FillRectangle(0, headerY, w, t.fontHeight, colorHeaderBG)
	t.drawStringClipped(0, headerY, t.headerText(), colorFG, t.cols)

	panelY := t.fontHeight
	viewH := int16(t.viewRows) * t.fontHeight
	_ = t.d.FillRectangle(0, panelY, w, viewH, colorPanelBG)

	switch t.mode {
	case modeGraph:
		t.renderGraph(panelY, w, int(viewH))
	default:
		t.renderHistory(panelY)
	}

	statusY := int16(t.rows-1) * t.fontHeight
	_ = t.d.FillRectangle(0, statusY, w, t.fontHeight, colorStatusBG)
	t.drawStringClipped(0, statusY, t.statusText(), colorFG, t.cols)

	if t.showHelp {
		t.renderHelp()
	}

	_ = t.fb.Present()
}

func (t *Task) renderHistory(panelY int16) {
	maxLines := t.viewRows
	start := 0
	if len(t.lines) > maxLines {
		start = len(t.lines) - maxLines
	}
	y := panelY
	for i := start; i < len(t.lines); i++ {
		t.drawStringClipped(0, y, t.lines[i], colorFG, t.cols)
		y += t.fontHeight
	}
}

func (t *Task) renderGraph(panelY int16, w int16, viewHPx int) {
	if t.graph == nil && len(t.plots) == 0 {
		t.drawStringClipped(0, panelY, "graph: no expression (enter `sin(x)` and press Enter)", colorDim, t.cols)
		return
	}

	px0 := int16(0)
	py0 := panelY
	pw := w
	ph := int16(viewHPx)

	_ = t.d.FillRectangle(px0, py0, pw, ph, colorBG)

	t.drawGrid(px0, py0, pw, ph)
	t.drawAxes(px0, py0, pw, ph)
	t.drawPlots(px0, py0, pw, ph)
}

func (t *Task) drawGrid(px0, py0, pw, ph int16) {
	_ = px0
	_ = py0
	_ = pw
	_ = ph
}

func (t *Task) drawAxes(px0, py0, pw, ph int16) {
	if t.xMin >= t.xMax || t.yMin >= t.yMax {
		return
	}
	if t.xMin <= 0 && t.xMax >= 0 {
		x := int16((0 - t.xMin) / (t.xMax - t.xMin) * float64(pw-1))
		for y := int16(0); y < ph; y++ {
			t.d.SetPixel(px0+x, py0+y, colorAxis)
		}
	}
	if t.yMin <= 0 && t.yMax >= 0 {
		y := int16((t.yMax - 0) / (t.yMax - t.yMin) * float64(ph-1))
		for x := int16(0); x < pw; x++ {
			t.d.SetPixel(px0+x, py0+y, colorAxis)
		}
	}
}

func (t *Task) drawPlots(px0, py0, pw, ph int16) {
	if t.xMin >= t.xMax || t.yMin >= t.yMax {
		return
	}
	if pw <= 1 || ph <= 1 {
		return
	}

	plots := t.plots
	if len(plots) == 0 && t.graph != nil {
		plots = []plot{{src: t.graphExpr, expr: t.graph}}
	}

	colors := []color.RGBA{colorPlot0, colorPlot1, colorPlot2, colorPlot3}
	for i, p := range plots {
		c := colors[i%len(colors)]
		if len(p.xs) != 0 && len(p.ys) == len(p.xs) {
			t.drawPlotSeries(px0, py0, pw, ph, p.xs, p.ys, c)
		} else {
			t.drawPlotFunc(px0, py0, pw, ph, p.expr, c)
		}
	}
}

func (t *Task) drawPlotFunc(px0, py0, pw, ph int16, expr node, c color.RGBA) {
	if expr == nil {
		return
	}
	prevOK := false
	var prevX, prevY int16
	for ix := int16(0); ix < pw; ix++ {
		x := t.xMin + (float64(ix)/float64(pw-1))*(t.xMax-t.xMin)
		y, ok := t.evalGraphFor(expr, x)
		if !ok || math.IsNaN(y) || math.IsInf(y, 0) {
			prevOK = false
			continue
		}
		iy := int16((t.yMax - y) / (t.yMax - t.yMin) * float64(ph-1))
		if iy < 0 || iy >= ph {
			prevOK = false
			continue
		}

		if prevOK {
			t.drawLine(px0+prevX, py0+prevY, px0+ix, py0+iy, c)
		} else {
			t.d.SetPixel(px0+ix, py0+iy, c)
		}
		prevOK = true
		prevX = ix
		prevY = iy
	}
}

func (t *Task) drawPlotSeries(px0, py0, pw, ph int16, xs, ys []float64, c color.RGBA) {
	if len(xs) == 0 || len(xs) != len(ys) {
		return
	}

	prevOK := false
	var prevX, prevY int16
	for i := range xs {
		x := xs[i]
		y := ys[i]
		if math.IsNaN(x) || math.IsInf(x, 0) || math.IsNaN(y) || math.IsInf(y, 0) {
			prevOK = false
			continue
		}

		ix := int16((x - t.xMin) / (t.xMax - t.xMin) * float64(pw-1))
		iy := int16((t.yMax - y) / (t.yMax - t.yMin) * float64(ph-1))
		if ix < 0 || ix >= pw || iy < 0 || iy >= ph {
			prevOK = false
			continue
		}
		if prevOK {
			t.drawLine(px0+prevX, py0+prevY, px0+ix, py0+iy, c)
		} else {
			t.d.SetPixel(px0+ix, py0+iy, c)
		}
		prevOK = true
		prevX = ix
		prevY = iy
	}
}

func (t *Task) drawLine(x0, y0, x1, y1 int16, c color.RGBA) {
	dx := int(math.Abs(float64(x1 - x0)))
	dy := -int(math.Abs(float64(y1 - y0)))
	sx := -1
	if x0 < x1 {
		sx = 1
	}
	sy := -1
	if y0 < y1 {
		sy = 1
	}
	err := dx + dy
	for {
		t.d.SetPixel(x0, y0, c)
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += int16(sx)
		}
		if e2 <= dx {
			err += dx
			y0 += int16(sy)
		}
	}
}

func (t *Task) renderHelp() {
	lines := []string{
		"Vector help",
		"",
		"Calculator",
		"  Enter: evaluate",
		"  a=...: assign variable",
		"  f(x)=...: define function",
		"  simp(expr): simplify",
		"  diff(expr, x): derivative",
		"  :exact / :float: eval mode",
		"  :prec N: float format",
		"  :plotclear: clear plots",
		"  H: toggle help",
		"  g: toggle graph (last expr)",
		"  q/ESC: exit",
		"",
		"Graph",
		"  arrows: pan",
		"  +/-: zoom in/out",
		"  a: autoscale y",
		"  c: back to calculator",
	}

	boxCols := t.cols - 4
	boxRows := t.rows - 4
	if boxCols < 24 || boxRows < 10 {
		return
	}
	innerCols := boxCols - 2
	contentRows := boxRows - 3

	if t.helpTop < 0 {
		t.helpTop = 0
	}
	maxTop := len(lines) - contentRows
	if maxTop < 0 {
		maxTop = 0
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

	title := "Vector  (H/Esc close, Up/Down scroll)"
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
		if lines[i] == "" || lines[i] == "Calculator" || lines[i] == "Graph" {
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
