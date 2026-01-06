package vector

import (
	"fmt"
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

	switch t.tab {
	case tabPlot:
		t.renderGraph(panelY, w, int(viewH))
	case tabStack:
		t.renderStack(panelY)
	default:
		t.renderTerminal(panelY)
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
		t.drawHighlightedLine(0, y, t.lines[i], t.cols)
		y += t.fontHeight
	}
}

func (t *Task) renderTerminal(panelY int16) {
	histRows := t.viewRows - 1
	if histRows < 0 {
		histRows = 0
	}

	start := 0
	if len(t.lines) > histRows {
		start = len(t.lines) - histRows
	}
	y := panelY
	for i := start; i < len(t.lines); i++ {
		t.drawHighlightedLine(0, y, t.lines[i], t.cols)
		y += t.fontHeight
	}

	inputY := panelY + int16(t.viewRows-1)*t.fontHeight
	if t.viewRows <= 0 {
		inputY = panelY
	}
	_ = t.d.FillRectangle(0, inputY, int16(t.cols)*t.fontWidth, t.fontHeight, colorBG)

	prefix := []rune("> ")
	visibleCols := t.cols - len(prefix)
	if visibleCols < 0 {
		visibleCols = 0
	}

	startCol := 0
	if t.cursor > visibleCols-1 {
		startCol = t.cursor - (visibleCols - 1)
		if startCol < 0 {
			startCol = 0
		}
	}
	if startCol > len(t.input) {
		startCol = len(t.input)
	}

	endCol := startCol + visibleCols
	if endCol > len(t.input) {
		endCol = len(t.input)
	}

	t.drawRunesClipped(0, inputY, prefix, colorDim, t.cols)
	t.drawHighlightedInput(int16(len(prefix))*t.fontWidth, inputY, t.input[startCol:endCol], colorFG, visibleCols)

	cursorCol := len(prefix) + (t.cursor - startCol)
	if cursorCol < len(prefix) {
		cursorCol = len(prefix)
	}
	if cursorCol > t.cols-1 {
		cursorCol = t.cols - 1
	}

	cursorX := int16(cursorCol) * t.fontWidth
	_ = t.d.FillRectangle(cursorX, inputY, t.fontWidth, t.fontHeight, colorHeaderBG)
	var cursorRune rune = ' '
	if t.cursor >= startCol && t.cursor < endCol {
		cursorRune = t.input[t.cursor]
	}
	t.drawRunesClipped(cursorX, inputY, []rune{cursorRune}, colorFG, 1)

	if t.cursor == len(t.input) && endCol == len(t.input) && t.ghost != "" && cursorCol+1 < t.cols {
		ghostCols := t.cols - (cursorCol + 1)
		t.drawStringClipped(int16(cursorCol+1)*t.fontWidth, inputY, t.ghost, colorDim, ghostCols)
	}
}

func (t *Task) drawHighlightedInput(x, y int16, rs []rune, fg color.RGBA, cols int) {
	t.drawHighlightedRunes(x, y, rs, fg, cols, true)
}

func (t *Task) drawHighlightedLine(x, y int16, s string, cols int) {
	if cols <= 0 {
		return
	}
	t.drawHighlightedRunes(x, y, []rune(s), colorFG, cols, false)
}

func (t *Task) drawHighlightedRunes(x, y int16, rs []rune, fg color.RGBA, cols int, isInput bool) {
	col := 0
	i := 0
	for i < len(rs) {
		if col >= cols {
			return
		}

		r := rs[i]
		switch {
		case r == ':' && i == 0:
			tinyfont.DrawChar(t.d, t.font, x+int16(col)*t.fontWidth, y+t.fontOffset, r, colorDim)
			col++
			i++
			cmdStart := i
			for i < len(rs) && isIdentContinue(rs[i]) {
				i++
			}
			t.drawRunesClipped(x+int16(col)*t.fontWidth, y, rs[cmdStart:i], colorPlot1, cols-col)
			col += i - cmdStart
			continue
		case isIdentStart(r):
			start := i
			i++
			for i < len(rs) && isIdentContinue(rs[i]) {
				i++
			}
			word := string(rs[start:i])
			c := fg
			if isKeyword(word) {
				c = colorPlot2
			}
			t.drawRunesClipped(x+int16(col)*t.fontWidth, y, rs[start:i], c, cols-col)
			col += i - start
			continue
		case (r >= '0' && r <= '9') || (r == '.' && i+1 < len(rs) && rs[i+1] >= '0' && rs[i+1] <= '9'):
			start := i
			i++
			for i < len(rs) && ((rs[i] >= '0' && rs[i] <= '9') || rs[i] == '.') {
				i++
			}
			if i < len(rs) && (rs[i] == 'e' || rs[i] == 'E') {
				j := i + 1
				if j < len(rs) && (rs[j] == '+' || rs[j] == '-') {
					j++
				}
				k := j
				for k < len(rs) && rs[k] >= '0' && rs[k] <= '9' {
					k++
				}
				if k > j {
					i = k
				}
			}
			t.drawRunesClipped(x+int16(col)*t.fontWidth, y, rs[start:i], colorPlot1, cols-col)
			col += i - start
			continue
		case r == '+' || r == '-' || r == '*' || r == '/' || r == '^' || r == '=' || r == ',' || r == '(' || r == ')' || r == ';':
			tinyfont.DrawChar(t.d, t.font, x+int16(col)*t.fontWidth, y+t.fontOffset, r, colorDim)
			col++
			i++
			continue
		default:
			tinyfont.DrawChar(t.d, t.font, x+int16(col)*t.fontWidth, y+t.fontOffset, r, fg)
			col++
			i++
			continue
		}
	}
}

func isKeyword(s string) bool {
	switch s {
	case "range", "simp", "diff",
		"sin", "cos", "tan", "asin", "acos", "atan",
		"sqrt", "abs", "exp", "ln", "min", "max":
		return true
	default:
		return false
	}
}

func (t *Task) renderStack(panelY int16) {
	vars := t.stackVars()
	if len(vars) == 0 {
		t.drawStringClipped(0, panelY, "stack: no variables", colorDim, t.cols)
		return
	}

	if t.stackSel < 0 {
		t.stackSel = 0
	}
	if t.stackSel >= len(vars) {
		t.stackSel = len(vars) - 1
	}
	if t.stackTop < 0 {
		t.stackTop = 0
	}
	maxTop := len(vars) - t.viewRows
	if maxTop < 0 {
		maxTop = 0
	}
	if t.stackTop > maxTop {
		t.stackTop = maxTop
	}

	y := panelY
	end := t.stackTop + t.viewRows
	if end > len(vars) {
		end = len(vars)
	}
	for i := t.stackTop; i < end; i++ {
		name := vars[i]
		line := name + " = " + t.formatValue(t.e.vars[name])
		if i == t.stackSel {
			_ = t.d.FillRectangle(0, y, int16(t.cols)*t.fontWidth, t.fontHeight, colorHeaderBG)
			t.drawHighlightedLine(0, y, line, t.cols)
		} else {
			t.drawHighlightedLine(0, y, line, t.cols)
		}
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

	leftMargin := int16(7) * t.fontWidth
	bottomMargin := t.fontHeight + 1
	if leftMargin < 1 {
		leftMargin = 1
	}
	if bottomMargin < 1 {
		bottomMargin = 1
	}

	plotX := px0 + leftMargin
	plotY := py0 + 1
	plotW := pw - leftMargin - 1
	plotH := ph - bottomMargin - 2
	if plotW <= 2 || plotH <= 2 {
		return
	}

	_ = t.d.FillRectangle(plotX, plotY, plotW, plotH, colorPanelBG)
	t.drawGrid(plotX, plotY, plotW, plotH, leftMargin, bottomMargin)
	t.drawAxes(plotX, plotY, plotW, plotH)
	t.drawPlots(plotX, plotY, plotW, plotH)

	plots := t.plots
	if len(plots) == 0 && t.graph != nil {
		plots = []plot{{src: t.graphExpr, expr: t.graph}}
	}
	t.drawLegend(plotX, plotY, plotW, plotH, plots)
}

func (t *Task) drawGrid(plotX, plotY, plotW, plotH, leftMargin, bottomMargin int16) {
	if t.xMin >= t.xMax || t.yMin >= t.yMax {
		return
	}
	if plotW <= 2 || plotH <= 2 {
		return
	}

	xPxPerUnit := float64(plotW-1) / (t.xMax - t.xMin)
	yPxPerUnit := float64(plotH-1) / (t.yMax - t.yMin)
	if xPxPerUnit <= 0 || yPxPerUnit <= 0 || math.IsInf(xPxPerUnit, 0) || math.IsInf(yPxPerUnit, 0) {
		return
	}

	stepX := niceStep(40 / xPxPerUnit)
	stepY := niceStep(28 / yPxPerUnit)

	xStart := math.Ceil(t.xMin/stepX) * stepX
	for x := xStart; x <= t.xMax; x += stepX {
		ix := int16((x - t.xMin) / (t.xMax - t.xMin) * float64(plotW-1))
		for y := int16(0); y < plotH; y++ {
			t.d.SetPixel(plotX+ix, plotY+y, colorGrid)
		}
		label := fmtAxis(x)
		t.drawXAxisLabel(plotX+ix, plotY+plotH+1, label)
	}

	yStart := math.Ceil(t.yMin/stepY) * stepY
	for y := yStart; y <= t.yMax; y += stepY {
		iy := int16((t.yMax - y) / (t.yMax - t.yMin) * float64(plotH-1))
		for x := int16(0); x < plotW; x++ {
			t.d.SetPixel(plotX+x, plotY+iy, colorGrid)
		}
		label := fmtAxis(y)
		t.drawYAxisLabel(plotX-1, plotY+iy, label, leftMargin)
	}

	_ = bottomMargin
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

func (t *Task) drawLegend(px0, py0, pw, ph int16, plots []plot) {
	if len(plots) == 0 {
		return
	}
	if pw <= 2*t.fontWidth || ph <= t.fontHeight {
		return
	}

	plotCols := int(pw / t.fontWidth)
	if plotCols < 8 {
		return
	}

	maxEntries := int((ph - 2) / t.fontHeight)
	if maxEntries > 6 {
		maxEntries = 6
	}
	if maxEntries <= 0 {
		return
	}

	if len(plots) > maxEntries {
		plots = plots[:maxEntries]
	}

	boxCols := 26
	if plotCols-2 < boxCols {
		boxCols = plotCols - 2
	}
	if boxCols < 10 {
		boxCols = plotCols - 2
	}
	if boxCols < 8 {
		return
	}

	boxW := int16(boxCols) * t.fontWidth
	boxH := int16(len(plots))*t.fontHeight + 2
	if boxW > pw-2 {
		boxW = pw - 2
	}
	if boxH > ph-2 {
		boxH = ph - 2
	}

	x := px0 + 1
	y := py0 + 1

	_ = t.d.FillRectangle(x, y, boxW, boxH, colorHeaderBG)
	_ = t.d.FillRectangle(x, y, boxW, 1, colorAxis)
	_ = t.d.FillRectangle(x, y+boxH-1, boxW, 1, colorAxis)
	_ = t.d.FillRectangle(x, y, 1, boxH, colorAxis)
	_ = t.d.FillRectangle(x+boxW-1, y, 1, boxH, colorAxis)

	colors := []color.RGBA{colorPlot0, colorPlot1, colorPlot2, colorPlot3}
	swatchW := t.fontWidth * 2
	if swatchW < 6 {
		swatchW = 6
	}
	textX := x + 2 + swatchW + t.fontWidth
	textCols := int((boxW - (textX - x)) / t.fontWidth)
	if textCols < 1 {
		return
	}

	for i, p := range plots {
		c := colors[i%len(colors)]
		rowY := y + 1 + int16(i)*t.fontHeight

		_ = t.d.FillRectangle(x+2, rowY+t.fontHeight/2-1, swatchW, 3, c)

		label := p.src
		if label == "" {
			label = "plot"
		}
		t.drawStringClipped(textX, rowY, label, colorFG, textCols)
	}
}

func (t *Task) drawXAxisLabel(px, py int16, s string) {
	if s == "" {
		return
	}
	rs := []rune(s)
	w := int16(len(rs)) * t.fontWidth
	x := px - w/2
	if x < 0 {
		x = 0
	}
	maxCols := int((int16(t.cols)*t.fontWidth - x) / t.fontWidth)
	if maxCols <= 0 {
		return
	}
	t.drawStringClipped(x, py, s, colorDim, maxCols)
}

func (t *Task) drawYAxisLabel(rightEdgePx, py int16, s string, leftMargin int16) {
	if s == "" {
		return
	}
	rs := []rune(s)
	w := int16(len(rs)) * t.fontWidth
	x := rightEdgePx - w - 1
	minX := rightEdgePx - leftMargin + 1
	if x < minX {
		x = minX
	}
	if x < 0 {
		x = 0
	}
	maxCols := int((rightEdgePx - x) / t.fontWidth)
	if maxCols <= 0 {
		return
	}
	t.drawStringClipped(x, py-t.fontHeight/2, s, colorDim, maxCols)
}

func niceStep(raw float64) float64 {
	if raw <= 0 || math.IsNaN(raw) || math.IsInf(raw, 0) {
		return 1
	}
	pow := math.Pow(10, math.Floor(math.Log10(raw)))
	if pow == 0 || math.IsNaN(pow) || math.IsInf(pow, 0) {
		return 1
	}
	frac := raw / pow
	switch {
	case frac <= 1:
		return 1 * pow
	case frac <= 2:
		return 2 * pow
	case frac <= 5:
		return 5 * pow
	default:
		return 10 * pow
	}
}

func fmtAxis(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return ""
	}
	if math.Abs(v) < 1e-12 {
		return "0"
	}
	av := math.Abs(v)
	switch {
	case av >= 1000 || av < 0.01:
		return fmt.Sprintf("%.2g", v)
	case av >= 10:
		return fmt.Sprintf("%.0f", v)
	case av >= 1:
		return fmt.Sprintf("%.2f", v)
	default:
		return fmt.Sprintf("%.3f", v)
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
	var prevX, prevY float64
	xMin := 0.0
	yMin := 0.0
	xMax := float64(pw - 1)
	yMax := float64(ph - 1)
	for ix := int16(0); ix < pw; ix++ {
		x := t.xMin + (float64(ix)/float64(pw-1))*(t.xMax-t.xMin)
		y, ok := t.evalGraphFor(expr, x)
		if !ok || math.IsNaN(y) || math.IsInf(y, 0) {
			prevOK = false
			continue
		}

		curX := float64(ix)
		curY := (t.yMax - y) / (t.yMax - t.yMin) * float64(ph-1)
		if prevOK {
			cx0, cy0, cx1, cy1, ok := clipLineToRect(prevX, prevY, curX, curY, xMin, yMin, xMax, yMax)
			if ok {
				t.drawLine(
					px0+roundInt16(cx0),
					py0+roundInt16(cy0),
					px0+roundInt16(cx1),
					py0+roundInt16(cy1),
					c,
				)
			}
		} else if curY >= yMin && curY <= yMax {
			t.d.SetPixel(px0+ix, py0+roundInt16(curY), c)
		}
		prevOK = true
		prevX = curX
		prevY = curY
	}
}

func (t *Task) drawPlotSeries(px0, py0, pw, ph int16, xs, ys []float64, c color.RGBA) {
	if len(xs) == 0 || len(xs) != len(ys) {
		return
	}

	prevOK := false
	var prevX, prevY float64
	xMin := 0.0
	yMin := 0.0
	xMax := float64(pw - 1)
	yMax := float64(ph - 1)
	for i := range xs {
		x := xs[i]
		y := ys[i]
		if math.IsNaN(x) || math.IsInf(x, 0) || math.IsNaN(y) || math.IsInf(y, 0) {
			prevOK = false
			continue
		}

		curX := (x - t.xMin) / (t.xMax - t.xMin) * float64(pw-1)
		curY := (t.yMax - y) / (t.yMax - t.yMin) * float64(ph-1)
		if prevOK {
			cx0, cy0, cx1, cy1, ok := clipLineToRect(prevX, prevY, curX, curY, xMin, yMin, xMax, yMax)
			if ok {
				t.drawLine(
					px0+roundInt16(cx0),
					py0+roundInt16(cy0),
					px0+roundInt16(cx1),
					py0+roundInt16(cy1),
					c,
				)
			}
		} else if curX >= xMin && curX <= xMax && curY >= yMin && curY <= yMax {
			t.d.SetPixel(px0+roundInt16(curX), py0+roundInt16(curY), c)
		}
		prevOK = true
		prevX = curX
		prevY = curY
	}
}

func clipLineToRect(x0, y0, x1, y1, xmin, ymin, xmax, ymax float64) (cx0, cy0, cx1, cy1 float64, ok bool) {
	dx := x1 - x0
	dy := y1 - y0
	u1 := 0.0
	u2 := 1.0

	p := [4]float64{-dx, dx, -dy, dy}
	q := [4]float64{x0 - xmin, xmax - x0, y0 - ymin, ymax - y0}
	for i := 0; i < 4; i++ {
		if p[i] == 0 {
			if q[i] < 0 {
				return 0, 0, 0, 0, false
			}
			continue
		}
		t := q[i] / p[i]
		if p[i] < 0 {
			if t > u2 {
				return 0, 0, 0, 0, false
			}
			if t > u1 {
				u1 = t
			}
		} else {
			if t < u1 {
				return 0, 0, 0, 0, false
			}
			if t < u2 {
				u2 = t
			}
		}
	}

	cx0 = x0 + u1*dx
	cy0 = y0 + u1*dy
	cx1 = x0 + u2*dx
	cy1 = y0 + u2*dy
	if cx0 < xmin {
		cx0 = xmin
	}
	if cx0 > xmax {
		cx0 = xmax
	}
	if cx1 < xmin {
		cx1 = xmin
	}
	if cx1 > xmax {
		cx1 = xmax
	}
	if cy0 < ymin {
		cy0 = ymin
	}
	if cy0 > ymax {
		cy0 = ymax
	}
	if cy1 < ymin {
		cy1 = ymin
	}
	if cy1 > ymax {
		cy1 = ymax
	}
	return cx0, cy0, cx1, cy1, true
}

func roundInt16(v float64) int16 {
	if v < 0 {
		return int16(v - 0.5)
	}
	return int16(v + 0.5)
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
		"Tabs",
		"  F1: terminal (REPL)",
		"  F2: plot",
		"  F3: stack (variables)",
		"",
		"Terminal",
		"  Enter: evaluate",
		"  a=...: assign variable",
		"  f(x)=...: define function",
		"  simp(expr): simplify",
		"  diff(expr, x): derivative",
		"  :exact / :float: eval mode",
		"  :prec N: float format",
		"  :plotclear: clear plots",
		"  :clear: clear output history",
		"  Ctrl+G: jump to plot tab",
		"  H: toggle help",
		"  q/ESC: exit",
		"",
		"Plot",
		"  arrows: pan",
		"  +/-: zoom in/out",
		"  PgUp/PgDn: zoom",
		"  z: cycle zoom step",
		"  a: autoscale",
		"  c: back to terminal",
		"",
		"Stack",
		"  Up/Down: select",
		"  Enter/e: edit value",
		"  Enter: apply (in editor)",
		"  Esc: cancel edit",
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
