package gpioscope

import (
	"fmt"
	"image/color"

	"spark/sparkos/proto"
)

var (
	colorHotbarSelBG = color.RGBA{R: 0x00, G: 0x60, B: 0xD0, A: 0xFF}
	colorHotbarSelFG = color.RGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
	colorBoxBorder   = color.RGBA{R: 0x30, G: 0x30, B: 0x30, A: 0xFF}
)

func (t *Task) renderUI() {
	w := int16(t.fb.Width())
	h := int16(t.fb.Height())
	t.d.FillRectangle(0, 0, w, h, colorBG)

	headerH := t.fontHeight + 2
	footerH := t.fontHeight*2 + 3

	rightCols := int16(24)
	rightPx := rightCols * t.fontWidth
	if rightPx > w-8*t.fontWidth {
		rightPx = w - 8*t.fontWidth
	}
	if rightPx < 16*t.fontWidth {
		rightPx = 16 * t.fontWidth
	}
	mainW := w - rightPx
	if mainW < 8*t.fontWidth {
		mainW = 8 * t.fontWidth
		rightPx = w - mainW
	}

	t.renderHotbar(0, 0, w, headerH)

	mainX := int16(0)
	mainY := headerH
	mainH := h - headerH - footerH
	if mainH < t.fontHeight*4 {
		mainH = t.fontHeight * 4
	}
	sideX := mainW
	sideY := headerH
	sideW := rightPx
	sideH := h - headerH - footerH

	t.renderMain(mainX, mainY, mainW, mainH)
	t.renderSide(sideX, sideY, sideW, sideH)
	t.renderFooter(0, h-footerH, w, footerH)

	if t.showMenu {
		t.renderMenuOverlay(mainX, headerH, w, h-footerH)
	}
	if t.showHelp {
		t.renderHelpOverlay(mainX, headerH, w, h-footerH)
	}

	_ = t.fb.Present()
}

func (t *Task) renderHotbar(x, y, w, h int16) {
	t.d.FillRectangle(x, y, w, h, colorHeaderBG)

	cats := t.menuCats()
	px := x + 2
	py := y + t.fontOffset + 1

	for _, c := range cats {
		name := t.menuCatName(c)
		boxW := int16(len(name)+2) * t.fontWidth
		bg := colorHeaderBG
		fg := colorFG
		if t.showMenu && t.menuCat == c {
			bg = colorHotbarSelBG
			fg = colorHotbarSelFG
		}
		if px+boxW > x+w {
			break
		}
		t.d.FillRectangle(px-1, y+1, boxW, h-2, bg)
		writeText(t.d, t.font, px, py, fg, name)
		px += boxW + t.fontWidth
	}

	title := "GPIO Scope"
	if px+int16(len(title))*t.fontWidth+2 < x+w {
		writeText(t.d, t.font, x+w-int16(len(title))*t.fontWidth-2, py, colorDim, title)
	}
}

func (t *Task) renderFooter(x, y, w, h int16) {
	t.d.FillRectangle(x, y, w, h, colorHeaderBG)

	modeName := "GPIO"
	switch t.mode {
	case modeSignal:
		modeName = "Signal"
	case modeProtocol:
		modeName = "Protocol"
	}
	run := "STOP"
	if t.running {
		run = "RUN"
	}
	trig := "trig:none"
	if t.triggerArmed {
		trig = fmt.Sprintf("trig:%s@%d", t.triggerName(), t.triggerPinID)
	}
	line0 := fmt.Sprintf("MODE:%s  CAP:%s  %s  zoom:%dx  %s", modeName, run, fmtHz(t.periodTicks), t.samplesPerPx, trig)
	writeText(t.d, t.font, x+2, y+t.fontOffset, colorFG, fitText(line0, t.cols))

	line1 := t.msg
	if t.showMenu {
		line1 = "menu: ←→ cat  ↑↓ item  Enter apply  Esc close"
	} else if t.showHelp {
		line1 = "help: Esc close"
	}
	writeText(t.d, t.font, x+2, y+t.fontHeight+t.fontOffset+1, colorDim, fitText(line1, t.cols))
}

func (t *Task) renderMain(x, y, w, h int16) {
	t.drawBox(x, y, w, h, t.mainTitle())
	innerX := x + 1
	innerY := y + t.fontHeight + 2
	innerW := w - 2
	innerH := h - (t.fontHeight + 3)
	if innerW <= 0 || innerH <= 0 {
		return
	}

	switch t.mode {
	case modeGPIO:
		t.renderPinsList(innerX, innerY, innerW, innerH, true)
	default:
		t.renderWaveArea(innerX, innerY, innerW, innerH)
	}
}

func (t *Task) mainTitle() string {
	switch t.mode {
	case modeSignal:
		return "Signals"
	case modeProtocol:
		return "Signals + Decode"
	default:
		return "Pins"
	}
}

func (t *Task) renderSide(x, y, w, h int16) {
	t.drawBox(x, y, w, h, "Control")
	innerX := x + 2
	innerY := y + t.fontHeight + 2
	innerW := w - 4
	innerH := h - (t.fontHeight + 4)
	if innerW <= 0 || innerH <= 0 {
		return
	}

	row := int16(0)
	writeText(t.d, t.font, innerX, innerY+row+t.fontOffset, colorDim, "Capture")
	row += t.fontHeight
	writeText(t.d, t.font, innerX, innerY+row+t.fontOffset, colorFG, fmt.Sprintf("run: %v", t.running))
	row += t.fontHeight
	writeText(t.d, t.font, innerX, innerY+row+t.fontOffset, colorFG, fmt.Sprintf("freeze: %v", t.frozenActive))
	row += t.fontHeight
	writeText(t.d, t.font, innerX, innerY+row+t.fontOffset, colorFG, fmt.Sprintf("period: %s", fmtHz(t.periodTicks)))
	row += t.fontHeight
	writeText(t.d, t.font, innerX, innerY+row+t.fontOffset, colorFG, fmt.Sprintf("zoom: %dx", t.samplesPerPx))
	row += t.fontHeight + 2

	writeText(t.d, t.font, innerX, innerY+row+t.fontOffset, colorDim, "Signal")
	row += t.fontHeight
	writeText(t.d, t.font, innerX, innerY+row+t.fontOffset, colorFG, fmt.Sprintf("gen: %v", t.sigGenActive))
	row += t.fontHeight
	writeText(t.d, t.font, innerX, innerY+row+t.fontOffset, colorFG, fmt.Sprintf("pin: %d", t.sigGenPinID))
	row += t.fontHeight
	writeText(t.d, t.font, innerX, innerY+row+t.fontOffset, colorFG, fmt.Sprintf("half: %s", fmtHz(t.sigGenHalfPeriodTicks)))
	row += t.fontHeight + 2

	writeText(t.d, t.font, innerX, innerY+row+t.fontOffset, colorDim, "Trigger")
	row += t.fontHeight
	writeText(t.d, t.font, innerX, innerY+row+t.fontOffset, colorFG, fmt.Sprintf("%s @ %d", t.triggerName(), t.triggerPinID))
	row += t.fontHeight
	writeText(t.d, t.font, innerX, innerY+row+t.fontOffset, colorFG, fmt.Sprintf("armed: %v", t.triggerArmed))
	row += t.fontHeight + 2

	writeText(t.d, t.font, innerX, innerY+row+t.fontOffset, colorDim, "Watched")
	row += t.fontHeight
	count := 0
	for _, p := range t.pins {
		if p.selected {
			count++
		}
	}
	writeText(t.d, t.font, innerX, innerY+row+t.fontOffset, colorFG, fmt.Sprintf("%d pins", count))
	row += t.fontHeight + 2

	if row < innerH {
		writeText(t.d, t.font, innerX, innerY+row+t.fontOffset, colorDim, "Tip: m opens menu")
	}
}

func (t *Task) renderPinsList(x, y, w, h int16, showDetails bool) {
	maxRows := int(h / t.fontHeight)
	if maxRows < 1 {
		return
	}

	top := 0
	if t.sel >= maxRows {
		top = t.sel - maxRows + 1
	}
	for i := 0; i < maxRows && top+i < len(t.pins); i++ {
		p := t.pins[top+i]
		py := y + int16(i)*t.fontHeight
		bg := colorBG
		fg := colorFG
		if top+i == t.sel {
			bg = colorSelBG
			fg = colorSelFG
			t.d.FillRectangle(x, py, w, t.fontHeight, bg)
		}

		sel := ' '
		if p.selected {
			sel = '*'
		}
		m := "IN"
		if p.mode == proto.GPIOModeOutput {
			m = "OUT"
		}
		l := '0'
		if p.level {
			l = '1'
		}
		line := fmt.Sprintf("%c GPIO%02d  %-3s  %c", sel, p.id, m, l)
		if showDetails {
			line = fmt.Sprintf("%c GPIO%02d  %-3s  %c  caps:%v", sel, p.id, m, l, p.caps)
		}
		writeText(t.d, t.font, x+2, py+t.fontOffset, fg, fitText(line, int(w/t.fontWidth)-1))
	}
}

func (t *Task) renderWaveArea(x0, y0, w, h int16) {
	if t.showGrid {
		t.renderGrid(x0, y0, w, h)
	}

	var watch []pin
	for _, p := range t.pins {
		if p.selected {
			watch = append(watch, p)
		}
	}
	if len(watch) == 0 {
		writeText(t.d, t.font, x0+2, y0+t.fontOffset, colorDim, "No watched pins. Use menu: Capture -> (toggle) or select pins.")
		return
	}

	total := 0
	var samples []uint32
	if t.frozenActive {
		samples = t.frozen
		total = len(samples)
	} else {
		total = t.buf.Len()
	}
	if total == 0 {
		writeText(t.d, t.font, x0+2, y0+t.fontOffset, colorDim, "No samples yet. Use menu: Capture -> Run.")
		return
	}

	spp := t.samplesPerPx
	if spp < 1 {
		spp = 1
	}
	visible := int(w) * spp
	end := total - t.scroll
	if end < 0 {
		end = 0
	}
	start := end - visible
	if start < 0 {
		start = 0
	}

	laneH := int(h) / len(watch)
	if laneH < 10 {
		laneH = 10
	}

	for li, p := range watch {
		lY := int(y0) + li*laneH
		hiY := int16(lY + 2)
		loY := int16(lY + laneH - 3)

		var last bool
		for xi := 0; xi < int(w); xi++ {
			si := start + xi*spp
			if si >= end {
				break
			}
			level := t.sampleAt(samples, si, p.id)
			yy := loY
			if level {
				yy = hiY
			}
			if xi == 0 {
				last = level
			}
			if level != last {
				t.d.FillRectangle(x0+int16(xi), hiY, 1, loY-hiY+1, colorDim)
			}
			last = level
			c := colorWaveLo
			if level {
				c = colorWaveHi
			}
			t.d.FillRectangle(x0+int16(xi), yy, 1, 2, c)
		}

		label := fmt.Sprintf("GPIO%02d", p.id)
		writeText(t.d, t.font, x0+2, int16(lY)+t.fontOffset, colorDim, fitText(label, int(w/t.fontWidth)-1))
	}

	if t.cursor >= 0 {
		cursor := t.cursor
		vis := end - start
		if vis < 0 {
			vis = 0
		}
		if cursor >= vis {
			cursor = vis - 1
		}
		if cursor >= 0 {
			cx := x0 + int16(cursor/spp)
			if cx >= x0 && cx < x0+w {
				t.d.FillRectangle(cx, y0, 1, h, colorCursor)
			}
		}
	}

	if t.mode == modeProtocol && len(t.decoded) > 0 {
		lines := joinTrimLines(t.decoded, 12)
		blockH := int16(len(lines))*t.fontHeight + 2
		if blockH < h {
			ty := y0 + h - blockH
			t.d.FillRectangle(x0, ty, w, blockH, colorHeaderBG)
			for i, s := range lines {
				writeText(t.d, t.font, x0+2, ty+int16(i)*t.fontHeight+t.fontOffset, colorFG, fitText(s, int(w/t.fontWidth)-1))
			}
		}
	}
}

func (t *Task) renderGrid(x, y, w, h int16) {
	step := int16(25)
	for gx := x; gx < x+w; gx += step {
		t.d.FillRectangle(gx, y, 1, h, colorPanelBG)
	}
	for gy := y; gy < y+h; gy += step {
		t.d.FillRectangle(x, gy, w, 1, colorPanelBG)
	}
}

func (t *Task) drawBox(x, y, w, h int16, title string) {
	if w <= 0 || h <= 0 {
		return
	}
	t.d.FillRectangle(x, y, w, h, colorPanelBG)
	t.d.FillRectangle(x, y, w, 1, colorBoxBorder)
	t.d.FillRectangle(x, y+h-1, w, 1, colorBoxBorder)
	t.d.FillRectangle(x, y, 1, h, colorBoxBorder)
	t.d.FillRectangle(x+w-1, y, 1, h, colorBoxBorder)
	t.d.FillRectangle(x+1, y+1, w-2, t.fontHeight+1, colorHeaderBG)
	writeText(t.d, t.font, x+2, y+t.fontOffset+1, colorFG, fitText(title, int(w/t.fontWidth)-2))
}

func (t *Task) renderMenuOverlay(x0, y0, w, h int16) {
	_ = x0
	_ = w
	_ = h

	items := t.menuItems(t.menuCat)
	if len(items) == 0 {
		return
	}
	if t.menuSel < 0 {
		t.menuSel = 0
	}
	if t.menuSel >= len(items) {
		t.menuSel = len(items) - 1
	}

	x := int16(2)
	for _, c := range t.menuCats() {
		if c == t.menuCat {
			break
		}
		name := t.menuCatName(c)
		x += int16(len(name)+2)*t.fontWidth + t.fontWidth
	}
	y := y0 + t.fontHeight + 2

	maxLabel := 0
	for _, it := range items {
		if l := len(it.Label); l > maxLabel {
			maxLabel = l
		}
	}
	boxW := int16(maxLabel+2) * t.fontWidth
	boxH := int16(len(items))*t.fontHeight + 2
	if boxW < 10*t.fontWidth {
		boxW = 10 * t.fontWidth
	}

	t.d.FillRectangle(x, y, boxW, boxH, colorHeaderBG)
	t.d.FillRectangle(x, y, boxW, 1, colorBoxBorder)
	t.d.FillRectangle(x, y+boxH-1, boxW, 1, colorBoxBorder)
	t.d.FillRectangle(x, y, 1, boxH, colorBoxBorder)
	t.d.FillRectangle(x+boxW-1, y, 1, boxH, colorBoxBorder)

	for i, it := range items {
		iy := y + 1 + int16(i)*t.fontHeight
		bg := colorHeaderBG
		fg := colorFG
		if i == t.menuSel {
			bg = colorHotbarSelBG
			fg = colorHotbarSelFG
		}
		t.d.FillRectangle(x+1, iy, boxW-2, t.fontHeight, bg)
		writeText(t.d, t.font, x+2, iy+t.fontOffset, fg, fitText(it.Label, int(boxW/t.fontWidth)-1))
	}
}

func (t *Task) renderHelpOverlay(_, y0, w, h int16) {
	lines := []string{
		"GPIO Scope",
		"",
		"Controls:",
		"  m        open menu",
		"  arrows   navigate pins / menu",
		"  Enter    apply menu item",
		"  Esc      close menu/help, or exit",
		"  q        exit",
		"",
		"Tip: Most actions are in the menu (top bar).",
	}

	max := 0
	for _, s := range lines {
		if len(s) > max {
			max = len(s)
		}
	}
	boxW := int16(max+4) * t.fontWidth
	boxH := int16(len(lines))*t.fontHeight + 4
	if boxW > w-4 {
		boxW = w - 4
	}
	if boxH > h-4 {
		boxH = h - 4
	}
	x := (w - boxW) / 2
	y := y0 + (h-boxH)/2

	t.d.FillRectangle(x, y, boxW, boxH, colorHeaderBG)
	t.d.FillRectangle(x, y, boxW, 1, colorBoxBorder)
	t.d.FillRectangle(x, y+boxH-1, boxW, 1, colorBoxBorder)
	t.d.FillRectangle(x, y, 1, boxH, colorBoxBorder)
	t.d.FillRectangle(x+boxW-1, y, 1, boxH, colorBoxBorder)

	for i, s := range lines {
		yy := y + 2 + int16(i)*t.fontHeight
		writeText(t.d, t.font, x+2, yy+t.fontOffset, colorFG, fitText(s, int(boxW/t.fontWidth)-2))
	}
}
