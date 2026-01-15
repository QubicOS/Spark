package rfanalyzer

import (
	"fmt"
	"image/color"

	"spark/hal"
	"spark/sparkos/fonts/const2bitcolor"
	"spark/sparkos/fonts/dejavumono5"

	"tinygo.org/x/drivers"
	"tinygo.org/x/tinyfont"
)

var (
	colorBG        = color.RGBA{R: 0x00, G: 0x00, B: 0x00, A: 0xFF}
	colorPanelBG   = color.RGBA{R: 0x08, G: 0x08, B: 0x08, A: 0xFF}
	colorHeaderBG  = color.RGBA{R: 0x1C, G: 0x1C, B: 0x1C, A: 0xFF}
	colorStatusBG  = color.RGBA{R: 0x16, G: 0x16, B: 0x16, A: 0xFF}
	colorBorder    = color.RGBA{R: 0x2E, G: 0x2E, B: 0x2E, A: 0xFF}
	colorFG        = color.RGBA{R: 0xEE, G: 0xEE, B: 0xEE, A: 0xFF}
	colorDim       = color.RGBA{R: 0x8A, G: 0x8A, B: 0x8A, A: 0xFF}
	colorAccent    = color.RGBA{R: 0x4A, G: 0xD1, B: 0xFF, A: 0xFF}
	colorWarn      = color.RGBA{R: 0xFF, G: 0xD1, B: 0x4A, A: 0xFF}
	colorSelBG     = color.RGBA{R: 0xE8, G: 0xE8, B: 0xE8, A: 0xFF}
	colorSelFG     = color.RGBA{R: 0x11, G: 0x11, B: 0x11, A: 0xFF}
	colorFocusMark = color.RGBA{R: 0x20, G: 0xA0, B: 0xFF, A: 0xFF}
)

type rect struct {
	x int16
	y int16
	w int16
	h int16
}

func (r rect) inset(dx, dy int16) rect {
	nx := r.x + dx
	ny := r.y + dy
	nw := r.w - 2*dx
	nh := r.h - 2*dy
	if nw < 0 {
		nw = 0
	}
	if nh < 0 {
		nh = 0
	}
	return rect{x: nx, y: ny, w: nw, h: nh}
}

type layout struct {
	menu      rect
	toolbar   rect
	status1   rect
	status2   rect
	spectrum  rect
	waterfall rect
	rf        rect
	sniffer   rect
	proto     rect

	leftCols  int
	rightCols int
}

func (t *Task) computeLayout() layout {
	w := int16(t.fb.Width())
	h := int16(t.fb.Height())

	headerH := int16(headerRows) * t.fontHeight
	statusH := int16(statusRows) * t.fontHeight
	mainY := headerH

	leftCols := 40
	if leftCols < 20 {
		leftCols = 20
	}
	if leftCols > t.cols-10 {
		leftCols = t.cols - 10
		if leftCols < 20 {
			leftCols = t.cols / 2
		}
	}
	rightCols := t.cols - leftCols
	leftW := int16(leftCols) * t.fontWidth
	rightW := w - leftW

	spectrumRows := 16
	if spectrumRows > t.mainRows-8 {
		spectrumRows = t.mainRows / 3
	}
	if spectrumRows < 10 {
		spectrumRows = 10
	}

	waterfallRows := t.mainRows - spectrumRows
	if waterfallRows < 8 {
		waterfallRows = 8
		spectrumRows = t.mainRows - waterfallRows
	}

	controlRows := 16
	snifferRows := 19
	if controlRows+snifferRows > t.mainRows-8 {
		controlRows = t.mainRows / 3
		snifferRows = t.mainRows / 3
	}
	if controlRows < 10 {
		controlRows = 10
	}
	if snifferRows < 10 {
		snifferRows = 10
	}
	protoRows := t.mainRows - controlRows - snifferRows
	if protoRows < 8 {
		protoRows = 8
		if controlRows+snifferRows+protoRows > t.mainRows {
			snifferRows = t.mainRows - controlRows - protoRows
		}
	}

	menu := rect{x: 0, y: 0, w: w, h: t.fontHeight}
	toolbar := rect{x: 0, y: t.fontHeight, w: w, h: t.fontHeight}
	status1 := rect{x: 0, y: h - statusH, w: w, h: t.fontHeight}
	status2 := rect{x: 0, y: h - statusH + t.fontHeight, w: w, h: t.fontHeight}

	spectrum := rect{x: 0, y: mainY, w: leftW, h: int16(spectrumRows) * t.fontHeight}
	waterfall := rect{x: 0, y: mainY + spectrum.h, w: leftW, h: int16(waterfallRows) * t.fontHeight}

	rf := rect{x: leftW, y: mainY, w: rightW, h: int16(controlRows) * t.fontHeight}
	sniffer := rect{x: leftW, y: rf.y + rf.h, w: rightW, h: int16(snifferRows) * t.fontHeight}
	proto := rect{x: leftW, y: sniffer.y + sniffer.h, w: rightW, h: int16(protoRows) * t.fontHeight}

	return layout{
		menu:      menu,
		toolbar:   toolbar,
		status1:   status1,
		status2:   status2,
		spectrum:  spectrum,
		waterfall: waterfall,
		rf:        rf,
		sniffer:   sniffer,
		proto:     proto,
		leftCols:  leftCols,
		rightCols: rightCols,
	}
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

func (t *Task) initFont() bool {
	f := &dejavumono5.DejaVuSansMono5
	t.font = f

	h, off, err := const2bitcolor.ComputeTerminalMetrics(f)
	if err != nil {
		return false
	}
	t.fontHeight = h
	t.fontOffset = off

	_, outboxWidth := tinyfont.LineWidth(t.font, "0")
	t.fontWidth = int16(outboxWidth)
	return t.fontWidth > 0 && t.fontHeight > 0
}

func (t *Task) renderDirty() {
	if !t.active || t.fb == nil || t.d == nil {
		return
	}
	if t.dirty == 0 {
		return
	}

	l := t.computeLayout()
	full := (t.dirty & dirtyAll) == dirtyAll
	if full {
		_ = t.d.FillRectangle(0, 0, int16(t.fb.Width()), int16(t.fb.Height()), colorBG)
	}

	if full || (t.dirty&dirtyHeader) != 0 {
		t.renderHeader(l)
	}
	if full || (t.dirty&dirtySpectrum) != 0 {
		t.renderSpectrum(l)
	}
	if full || (t.dirty&dirtyWaterfall) != 0 {
		t.renderWaterfall(l)
	}
	if full || (t.dirty&dirtyRFControl) != 0 {
		t.renderRFControl(l)
	}
	if full || (t.dirty&dirtySniffer) != 0 {
		t.renderSniffer(l)
	}
	if full || (t.dirty&dirtyProtocol) != 0 {
		t.renderProtocol(l)
	}
	if full || (t.dirty&dirtyStatus) != 0 {
		t.renderStatus(l)
	}
	if full || (t.dirty&dirtyOverlay) != 0 {
		t.renderOverlay(l)
	}

	_ = t.fb.Present()
	t.dirty = 0
}

func (t *Task) renderHeader(l layout) {
	_ = t.d.FillRectangle(l.menu.x, l.menu.y, l.menu.w, l.menu.h, colorHeaderBG)
	_ = t.d.FillRectangle(l.toolbar.x, l.toolbar.y, l.toolbar.w, l.toolbar.h, colorHeaderBG)

	menu := "[m] View  RF  Capture  Decode  Display  Advanced  Help"
	if t.showMenu {
		menu = "[m] View  RF  Capture  Decode  Display  Advanced  Help  (open)"
	}
	t.drawStringClipped(l.menu.x+2, l.menu.y, menu, colorFG, t.cols)

	title := "2.4GHz RF Analyzer  nRF24 scan+spectrum+waterfall+sniffer"
	t.drawStringClipped(l.toolbar.x+2, l.toolbar.y, title, colorDim, t.cols)
}

func (t *Task) renderStatus(l layout) {
	_ = t.d.FillRectangle(l.status1.x, l.status1.y, l.status1.w, l.status1.h, colorStatusBG)
	_ = t.d.FillRectangle(l.status2.x, l.status2.y, l.status2.w, l.status2.h, colorStatusBG)

	mode := "IDLE"
	if t.scanActive {
		mode = "SCAN"
	}
	rfState := "RF:SIM"
	s1 := fmt.Sprintf("MODE:%s  CH:%03d  RATE:%s  PKT/s:%4d  DROP:%3d  %s", mode, t.selectedChannel, t.dataRate, 0, 0, rfState)
	t.drawStringClipped(l.status1.x+2, l.status1.y, s1, colorFG, t.cols)

	s2 := "keys: s scan  w wf  p cap  r reset  m menu  t focus  c chan  f filt  h help  q quit"
	t.drawStringClipped(l.status2.x+2, l.status2.y, s2, colorDim, t.cols)
}

func (t *Task) renderSpectrum(l layout) {
	t.renderPanel(l.spectrum, "Spectrum", t.focus == focusSpectrum)

	inner := l.spectrum.inset(2, 2)
	y0 := inner.y + t.fontHeight
	h := inner.h - t.fontHeight - 2
	if h <= 0 {
		return
	}

	// Placeholder "spectrum": a quiet gradient plus a channel marker.
	for x := int16(0); x < inner.w; x++ {
		level := uint8((x * 255) / (inner.w + 1))
		c := color.RGBA{R: 0x10, G: 0x10 + level/8, B: 0x10 + level/4, A: 0xFF}
		_ = t.d.FillRectangle(inner.x+x, y0, 1, h, c)
	}

	markerX := inner.x + int16((t.selectedChannel*int(inner.w))/126)
	_ = t.d.FillRectangle(markerX, y0, 1, h, colorAccent)

	info := fmt.Sprintf("ch %d  peak hold  avg", t.selectedChannel)
	t.drawStringClipped(inner.x+2, inner.y, info, colorFG, l.leftCols)
}

func (t *Task) renderWaterfall(l layout) {
	t.renderPanel(l.waterfall, "Waterfall", t.focus == focusWaterfall)

	inner := l.waterfall.inset(2, 2)
	y0 := inner.y + t.fontHeight
	h := inner.h - t.fontHeight - 2
	if h <= 0 {
		return
	}

	// Placeholder waterfall: static bands.
	for y := int16(0); y < h; y++ {
		level := uint8((y * 255) / (h + 1))
		c := color.RGBA{R: level / 3, G: level / 2, B: level, A: 0xFF}
		_ = t.d.FillRectangle(inner.x, y0+y, inner.w, 1, c)
	}

	state := "RUN"
	if t.waterfallFrozen {
		state = "FROZEN"
	}
	info := fmt.Sprintf("%s  speed:%d  sync:spectrum", state, t.scanSpeedScalar)
	t.drawStringClipped(inner.x+2, inner.y, info, colorFG, l.leftCols)
}

func (t *Task) renderRFControl(l layout) {
	t.renderPanel(l.rf, "RF Control", t.focus == focusRFControl)

	inner := l.rf.inset(2, 2)
	x := inner.x + 2
	y := inner.y

	lines := []string{
		fmt.Sprintf("CH LO : %3d", t.channelRangeLo),
		fmt.Sprintf("CH HI : %3d", t.channelRangeHi),
		fmt.Sprintf("DWELL : %3dms", t.dwellTimeMs),
		fmt.Sprintf("SPEED : %3d", t.scanSpeedScalar),
		fmt.Sprintf("RATE  : %s", t.dataRate),
		fmt.Sprintf("CRC   : %s", t.crcMode),
		fmt.Sprintf("ACK   : %v", t.autoAck),
		fmt.Sprintf("PWR   : %s", t.powerLevel),
	}

	maxRows := int((inner.h - t.fontHeight) / t.fontHeight)
	if maxRows < 1 {
		return
	}
	if len(lines) > maxRows {
		lines = lines[:maxRows]
	}

	y += t.fontHeight
	for i, line := range lines {
		fg := colorFG
		bg := colorPanelBG
		if i == t.selectedSetting && t.focus == focusRFControl {
			fg = colorSelFG
			bg = colorSelBG
		}
		_ = t.d.FillRectangle(inner.x+1, y+int16(i)*t.fontHeight, inner.w-2, t.fontHeight, bg)
		t.drawStringClipped(x, y+int16(i)*t.fontHeight, line, fg, l.rightCols-1)
	}
}

func (t *Task) renderSniffer(l layout) {
	t.renderPanel(l.sniffer, "Packet Sniffer", t.focus == focusSniffer)

	inner := l.sniffer.inset(2, 2)
	status := "LIVE"
	if t.capturePaused {
		status = "PAUSED"
	}
	hdr := fmt.Sprintf("%s  filt:%v", status, t.showFilters)
	t.drawStringClipped(inner.x+2, inner.y, hdr, colorFG, l.rightCols)

	body := inner.inset(1, 1)
	y := body.y + t.fontHeight
	msg := "(no packets yet)"
	t.drawStringClipped(body.x+2, y, msg, colorDim, l.rightCols)
}

func (t *Task) renderProtocol(l layout) {
	t.renderPanel(l.proto, "Protocol View", t.focus == focusProtocol)

	inner := l.proto.inset(2, 2)
	t.drawStringClipped(inner.x+2, inner.y, "raw/decoded  preamble addr payload crc", colorDim, l.rightCols)
	t.drawStringClipped(inner.x+2, inner.y+t.fontHeight, "(select a packet)", colorDim, l.rightCols)
}

func (t *Task) renderOverlay(l layout) {
	if t.showMenu {
		t.renderMenuOverlay(l)
		return
	}
	if t.showHelp {
		t.renderHelpOverlay(l)
		return
	}
	if t.showFilters {
		t.renderFiltersOverlay(l)
		return
	}
}

func (t *Task) renderMenuOverlay(l layout) {
	boxCols := t.cols - 4
	if boxCols > 52 {
		boxCols = 52
	}
	boxRows := 18
	px := int16(2) * t.fontWidth
	py := int16(headerRows) * t.fontHeight
	pw := int16(boxCols) * t.fontWidth
	ph := int16(boxRows) * t.fontHeight

	_ = t.d.FillRectangle(px, py, pw, ph, colorBorder)
	_ = t.d.FillRectangle(px+1, py+1, pw-2, ph-2, colorPanelBG)
	_ = t.d.FillRectangle(px+1, py+1, pw-2, t.fontHeight+1, colorHeaderBG)
	t.drawStringClipped(px+2, py, "Menu  (Esc/m close)", colorFG, boxCols)

	items := []string{
		"View     : spectrum/waterfall/sync",
		"RF       : range,dwell,speed,rate,crc,ack,pwr",
		"Capture  : start/stop,pause,drop policy",
		"Decode   : raw/decoded, addr len, esb",
		"Display  : wf palette, speed, theme",
		"Advanced : expert options, profiles",
		"Help     : hotkeys, about",
	}
	for i, it := range items {
		y := py + t.fontHeight + int16(i+1)*t.fontHeight
		t.drawStringClipped(px+2, y, it, colorDim, boxCols)
	}
}

func (t *Task) renderHelpOverlay(l layout) {
	boxCols := t.cols - 6
	if boxCols > 54 {
		boxCols = 54
	}
	boxRows := 16
	px := int16(3) * t.fontWidth
	py := int16(headerRows+2) * t.fontHeight
	pw := int16(boxCols) * t.fontWidth
	ph := int16(boxRows) * t.fontHeight

	_ = t.d.FillRectangle(px, py, pw, ph, colorBorder)
	_ = t.d.FillRectangle(px+1, py+1, pw-2, ph-2, colorPanelBG)
	_ = t.d.FillRectangle(px+1, py+1, pw-2, t.fontHeight+1, colorHeaderBG)
	t.drawStringClipped(px+2, py, "Help  (h/Esc close)", colorFG, boxCols)

	lines := []string{
		"s start/stop scan",
		"w freeze/resume waterfall",
		"p pause/resume packet capture",
		"r reset view",
		"m open menu",
		"t cycle focus",
		"c change channel",
		"f filters",
		"q quit",
		"",
		"Arrows: adjust focused panel",
	}
	for i, it := range lines {
		y := py + t.fontHeight + int16(i+1)*t.fontHeight
		fg := colorDim
		if it == "" {
			fg = colorBorder
		}
		t.drawStringClipped(px+2, y, it, fg, boxCols)
	}
}

func (t *Task) renderFiltersOverlay(l layout) {
	boxCols := l.rightCols
	if boxCols < 20 {
		boxCols = 20
	}
	boxRows := 12
	px := l.sniffer.x + 2
	py := l.sniffer.y + t.fontHeight*2
	pw := int16(boxCols) * t.fontWidth
	ph := int16(boxRows) * t.fontHeight

	_ = t.d.FillRectangle(px, py, pw, ph, colorBorder)
	_ = t.d.FillRectangle(px+1, py+1, pw-2, ph-2, colorPanelBG)
	_ = t.d.FillRectangle(px+1, py+1, pw-2, t.fontHeight+1, colorHeaderBG)
	t.drawStringClipped(px+2, py, "Filters (f close)", colorFG, boxCols)

	lines := []string{
		"CRC: any/ok/bad",
		"CH : all/sel/range",
		"LEN: min/max",
		"ADDR: match",
		"RATE: any/250K/1M/2M",
		"",
		"advanced: profiles",
	}
	for i, it := range lines {
		y := py + t.fontHeight + int16(i+1)*t.fontHeight
		t.drawStringClipped(px+2, y, it, colorDim, boxCols)
	}
}

func (t *Task) renderPanel(r rect, title string, focused bool) {
	_ = t.d.FillRectangle(r.x, r.y, r.w, r.h, colorBorder)
	inner := r.inset(1, 1)
	_ = t.d.FillRectangle(inner.x, inner.y, inner.w, inner.h, colorPanelBG)
	_ = t.d.FillRectangle(inner.x, inner.y, inner.w, t.fontHeight+1, colorHeaderBG)

	mark := " "
	markColor := colorDim
	if focused {
		mark = ">"
		markColor = colorFocusMark
	}
	t.drawStringClipped(inner.x+2, inner.y, mark, markColor, 1)
	t.drawStringClipped(inner.x+2+t.fontWidth, inner.y, title, colorFG, int(inner.w/t.fontWidth)-2)
}

func (t *Task) drawStringClipped(x, y int16, s string, fg color.RGBA, cols int) {
	col := int16(0)
	for _, r := range []rune(s) {
		if cols > 0 && int(col) >= cols {
			return
		}
		tinyfont.DrawChar(t.d, t.font, x+col*t.fontWidth, y+t.fontOffset, r, fg)
		col++
	}
}
