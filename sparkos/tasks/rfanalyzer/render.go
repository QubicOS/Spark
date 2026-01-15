package rfanalyzer

import (
	"fmt"
	"image/color"
	"strings"

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

	s1 := t.statusLine()
	if t.sweepCount != 0 {
		s1 = fmt.Sprintf("%s  SWP:%d", s1, t.sweepCount)
	}
	t.drawStringClipped(l.status1.x+2, l.status1.y, s1, colorFG, t.cols)

	s2 := "keys: s scan  w wf  p cap  r reset  m menu  t focus  c chan  f filt  h help  q quit"
	t.drawStringClipped(l.status2.x+2, l.status2.y, s2, colorDim, t.cols)
}

func (t *Task) renderSpectrum(l layout) {
	t.renderPanel(l.spectrum, "Spectrum", t.focus == focusSpectrum)

	inner := l.spectrum.inset(2, 2)
	headerY := inner.y
	labelsY := inner.y + inner.h - t.fontHeight
	plot := rect{
		x: inner.x,
		y: inner.y + t.fontHeight + 1,
		w: inner.w,
		h: inner.h - 2*t.fontHeight - 2,
	}
	if plot.h <= 0 || plot.w <= 0 {
		return
	}

	_ = t.d.FillRectangle(plot.x, plot.y, plot.w, plot.h, colorBG)

	for ch := 0; ch < numChannels; ch++ {
		x0 := plot.x + int16(ch)*plot.w/numChannels
		x1 := plot.x + int16(ch+1)*plot.w/numChannels
		if x1 <= x0 {
			x1 = x0 + 1
		}
		if x0 >= plot.x+plot.w {
			continue
		}
		if x1 > plot.x+plot.w {
			x1 = plot.x + plot.w
		}

		hAvg := int16(t.energyAvg[ch]) * plot.h / 255
		if hAvg > 0 {
			_ = t.d.FillRectangle(x0, plot.y+plot.h-hAvg, x1-x0, hAvg, colorAccent)
		}

		yPeak := plot.y + plot.h - 1 - int16(t.energyPeak[ch])*plot.h/255
		if yPeak >= plot.y && yPeak < plot.y+plot.h {
			_ = t.d.FillRectangle(x0, yPeak, x1-x0, 1, colorWarn)
		}
	}

	chLoX := plot.x + int16(t.channelRangeLo)*plot.w/numChannels
	chHiX := plot.x + int16(t.channelRangeHi+1)*plot.w/numChannels
	_ = t.d.FillRectangle(chLoX, plot.y, 1, plot.h, colorBorder)
	_ = t.d.FillRectangle(chHiX, plot.y, 1, plot.h, colorBorder)

	markerX := plot.x + int16(t.selectedChannel)*plot.w/numChannels
	_ = t.d.FillRectangle(markerX, plot.y, 1, plot.h, colorFG)

	for _, ch := range []int{0, 25, 50, 75, 100, 125} {
		x := plot.x + int16(ch)*plot.w/numChannels
		_ = t.d.FillRectangle(x, plot.y+plot.h-2, 1, 2, colorDim)
		label := fmt.Sprintf("%d", ch)
		t.drawStringClipped(x, labelsY, label, colorDim, l.leftCols)
	}

	info := fmt.Sprintf("ch %03d  avg+peak  rng %d-%d  dwell %dms  step %d", t.selectedChannel, t.channelRangeLo, t.channelRangeHi, t.dwellTimeMs, clampInt(t.scanSpeedScalar, 1, 10))
	t.drawStringClipped(inner.x+2, headerY, info, colorFG, l.leftCols)
}

func (t *Task) renderWaterfall(l layout) {
	t.renderPanel(l.waterfall, "Waterfall", t.focus == focusWaterfall)

	inner := l.waterfall.inset(2, 2)
	headerY := inner.y
	plot := rect{
		x: inner.x,
		y: inner.y + t.fontHeight + 1,
		w: inner.w,
		h: inner.h - t.fontHeight - 2,
	}
	if plot.h <= 0 || plot.w <= 0 {
		return
	}

	_ = t.d.FillRectangle(plot.x, plot.y, plot.w, plot.h, colorBG)
	if t.wfBuf != nil && t.wfW == int(plot.w) && t.wfH == int(plot.h) {
		t.blitWaterfall(plot)
	}

	markerX := plot.x + int16(t.selectedChannel)*plot.w/numChannels
	_ = t.d.FillRectangle(markerX, plot.y, 1, plot.h, colorFG)
	chLoX := plot.x + int16(t.channelRangeLo)*plot.w/numChannels
	chHiX := plot.x + int16(t.channelRangeHi+1)*plot.w/numChannels
	_ = t.d.FillRectangle(chLoX, plot.y, 1, plot.h, colorBorder)
	_ = t.d.FillRectangle(chHiX, plot.y, 1, plot.h, colorBorder)

	state := "RUN"
	if t.waterfallFrozen {
		state = "FROZEN"
	}
	info := fmt.Sprintf("%s  pal:%s  step:%d  sync:spectrum", state, t.wfPalette, clampInt(t.scanSpeedScalar, 1, 10))
	t.drawStringClipped(inner.x+2, headerY, info, colorFG, l.leftCols)
}

func (t *Task) renderRFControl(l layout) {
	t.renderPanel(l.rf, "RF Control", t.focus == focusRFControl)

	inner := l.rf.inset(2, 2)
	maxCols := int(inner.w / t.fontWidth)
	if maxCols <= 0 {
		return
	}

	y := inner.y + t.fontHeight
	preset := "PRESET: (none)"
	if t.activePreset != "" {
		preset = "PRESET: " + t.activePreset
		if t.presetDirty {
			preset += "*"
		}
	}
	t.drawStringClipped(inner.x+2, y, preset, colorDim, maxCols)
	y += t.fontHeight

	lines := []string{
		fmt.Sprintf("LO    [%03d]", t.channelRangeLo),
		fmt.Sprintf("HI    [%03d]", t.channelRangeHi),
		fmt.Sprintf("DWELL %s %02dms", sliderASCII(t.dwellTimeMs, 1, 50, 8), t.dwellTimeMs),
		fmt.Sprintf("STEP  %s %02d", sliderASCII(clampInt(t.scanSpeedScalar, 1, 10), 1, 10, 8), clampInt(t.scanSpeedScalar, 1, 10)),
		fmt.Sprintf("RATE  <%s>", t.dataRate),
		fmt.Sprintf("CRC   <%s>", t.crcMode),
		fmt.Sprintf("ACK   %s", checkboxASCII(t.autoAck)),
		fmt.Sprintf("PWR   <%s>", t.powerLevel),
	}

	for i, line := range lines {
		rowY := y + int16(i)*t.fontHeight
		if rowY+t.fontHeight > inner.y+inner.h {
			return
		}
		fg := colorFG
		bg := colorPanelBG
		if i == t.selectedSetting && t.focus == focusRFControl {
			fg = colorSelFG
			bg = colorSelBG
		}
		_ = t.d.FillRectangle(inner.x+1, rowY, inner.w-2, t.fontHeight, bg)
		t.drawStringClipped(inner.x+2, rowY, line, fg, maxCols)
	}
}

func (t *Task) renderSniffer(l layout) {
	t.renderPanel(l.sniffer, "Packet Sniffer", t.focus == focusSniffer)

	inner := l.sniffer.inset(2, 2)
	maxCols := int(inner.w / t.fontWidth)
	if maxCols <= 0 {
		return
	}

	status := "LIVE"
	if t.capturePaused {
		status = "PAUSED"
	}
	hdr := fmt.Sprintf("%s  pps:%d drop:%d  %s", status, t.pktsPerSec, t.pktDropped, t.filterSummary())
	t.drawStringClipped(inner.x+2, inner.y, hdr, colorFG, maxCols)

	colHdr := "t(ms) ch r ln addr   c"
	t.drawStringClipped(inner.x+2, inner.y+t.fontHeight, colHdr, colorDim, maxCols)

	listY := inner.y + 2*t.fontHeight
	listRows := int((inner.h - 2*t.fontHeight) / t.fontHeight)
	if listRows <= 0 {
		return
	}

	total := t.filteredCount()
	if total <= 0 {
		t.drawStringClipped(inner.x+2, listY, "(no packets)", colorDim, maxCols)
		return
	}
	if t.snifferSel < 0 {
		t.snifferSel = 0
	}
	if t.snifferSel >= total {
		t.snifferSel = total - 1
		if t.snifferSel < 0 {
			t.snifferSel = 0
		}
	}

	maxTop := total - listRows
	if maxTop < 0 {
		maxTop = 0
	}
	if t.snifferTop < 0 {
		t.snifferTop = 0
	}
	if t.snifferTop > maxTop {
		t.snifferTop = maxTop
	}
	if t.snifferSel < t.snifferTop {
		t.snifferTop = t.snifferSel
	}
	if t.snifferSel >= t.snifferTop+listRows {
		t.snifferTop = t.snifferSel - listRows + 1
		if t.snifferTop > maxTop {
			t.snifferTop = maxTop
		}
	}

	for row := 0; row < listRows; row++ {
		idx := t.snifferTop + row
		if idx < 0 || idx >= total {
			continue
		}
		p, ok := t.filteredPacketByIndex(idx)
		if !ok || p == nil {
			continue
		}

		ts := int(p.tick % 10000)
		r := rateShort(p.rate)
		line := fmt.Sprintf("%04d %03d %c %02d %s %s", ts, p.channel, r, p.length, p.addrSuffix3(), p.crcText())

		y := listY + int16(row)*t.fontHeight
		fg := colorFG
		bg := colorPanelBG
		if idx == t.snifferSel && t.focus == focusSniffer {
			fg = colorSelFG
			bg = colorSelBG
		}
		_ = t.d.FillRectangle(inner.x+1, y, inner.w-2, t.fontHeight, bg)
		t.drawStringClipped(inner.x+2, y, line, fg, maxCols)
	}
}

func (t *Task) renderProtocol(l layout) {
	t.renderPanel(l.proto, "Protocol View", t.focus == focusProtocol)

	inner := l.proto.inset(2, 2)
	maxCols := int(inner.w / t.fontWidth)
	if maxCols <= 0 {
		return
	}

	p, ok := t.filteredPacketByIndex(t.snifferSel)
	if !ok || p == nil {
		t.drawStringClipped(inner.x+2, inner.y, "mode:"+t.protoMode.String(), colorDim, maxCols)
		t.drawStringClipped(inner.x+2, inner.y+t.fontHeight, "(select a packet)", colorDim, maxCols)
		return
	}

	h1 := fmt.Sprintf("mode:%s  #%d  ch:%03d  t:%04d", t.protoMode, p.seq, p.channel, int(p.tick%10000))
	h2 := fmt.Sprintf("rate:%s len:%02d crc:%s addr:%dB", p.rate, p.length, p.crcText(), p.addrLen)
	t.drawStringClipped(inner.x+2, inner.y, h1, colorFG, maxCols)
	t.drawStringClipped(inner.x+2, inner.y+t.fontHeight, h2, colorDim, maxCols)

	y := inner.y + 2*t.fontHeight
	if y+t.fontHeight > inner.y+inner.h {
		return
	}

	if t.protoMode == protoRaw {
		var raw [1 + 5 + 32 + 2]byte
		n := 0
		raw[n] = 0x55
		n++
		for i := 0; i < int(p.addrLen) && i < len(p.addr); i++ {
			raw[n] = p.addr[i]
			n++
		}
		for i := 0; i < int(p.length) && i < len(p.payload); i++ {
			raw[n] = p.payload[i]
			n++
		}
		for i := 0; i < int(p.crcLen) && i < len(p.crc); i++ {
			raw[n] = p.crc[i]
			n++
		}
		t.renderHexDump(inner, y, raw[:n], maxCols)
		return
	}

	addrLine := fmt.Sprintf("pre:55 addr:%02X%02X%02X%02X%02X", p.addr[0], p.addr[1], p.addr[2], p.addr[3], p.addr[4])
	t.drawStringClipped(inner.x+2, y, addrLine, colorFG, maxCols)
	y += t.fontHeight

	if p.crcLen > 0 {
		crcLine := fmt.Sprintf("crc:%s %02X%02X", p.crcText(), p.crc[0], p.crc[1])
		t.drawStringClipped(inner.x+2, y, crcLine, colorFG, maxCols)
		y += t.fontHeight
	}

	t.renderHexDump(inner, y, p.payload[:p.length], maxCols)
}

func (t *Task) renderHexDump(box rect, y int16, data []byte, maxCols int) {
	// Format: "00: 0102030405060708"
	if len(data) == 0 {
		t.drawStringClipped(box.x+2, y, "(empty)", colorDim, maxCols)
		return
	}
	bytesPerLine := 8
	if bytesPerLine < 1 {
		bytesPerLine = 1
	}
	for off := 0; off < len(data); off += bytesPerLine {
		if y+t.fontHeight > box.y+box.h {
			return
		}
		chunk := data[off:]
		if len(chunk) > bytesPerLine {
			chunk = chunk[:bytesPerLine]
		}
		line := fmt.Sprintf("%02X: %s", off, hexBytes(chunk))
		t.drawStringClipped(box.x+2, y, line, colorDim, maxCols)
		y += t.fontHeight
	}
}

func hexBytes(b []byte) string {
	const digits = "0123456789ABCDEF"
	out := make([]byte, 0, len(b)*2)
	for _, v := range b {
		out = append(out, digits[(v>>4)&0x0F], digits[v&0x0F])
	}
	return string(out)
}

func (t *Task) renderOverlay(l layout) {
	if t.showPrompt {
		t.renderPromptOverlay(l)
		return
	}
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
	if boxCols > 56 {
		boxCols = 56
	}
	boxRows := 22
	if boxRows > t.mainRows {
		boxRows = t.mainRows
	}
	px := int16(2) * t.fontWidth
	py := int16(headerRows) * t.fontHeight
	pw := int16(boxCols) * t.fontWidth
	ph := int16(boxRows) * t.fontHeight

	_ = t.d.FillRectangle(px, py, pw, ph, colorBorder)
	_ = t.d.FillRectangle(px+1, py+1, pw-2, ph-2, colorPanelBG)
	_ = t.d.FillRectangle(px+1, py+1, pw-2, t.fontHeight+1, colorHeaderBG)

	catLine := t.menuCategoryLine()
	t.drawStringClipped(px+2, py, catLine, colorFG, boxCols)

	items := menuItems(t.menuCat)
	contentRows := boxRows - 3
	if contentRows < 1 {
		return
	}
	if t.menuSel < 0 {
		t.menuSel = 0
	}
	if t.menuSel >= len(items) {
		t.menuSel = len(items) - 1
		if t.menuSel < 0 {
			t.menuSel = 0
		}
	}
	top := t.menuSel - contentRows/2
	if top < 0 {
		top = 0
	}
	if top > len(items)-contentRows {
		top = len(items) - contentRows
	}
	if top < 0 {
		top = 0
	}

	for row := 0; row < contentRows; row++ {
		i := top + row
		if i < 0 || i >= len(items) {
			continue
		}
		y := py + t.fontHeight + int16(row+1)*t.fontHeight
		fg := colorFG
		bg := colorPanelBG
		if i == t.menuSel {
			fg = colorSelFG
			bg = colorSelBG
		}
		_ = t.d.FillRectangle(px+1, y, pw-2, t.fontHeight, bg)
		t.drawStringClipped(px+2, y, t.menuItemLine(items[i]), fg, boxCols)
	}
}

func (t *Task) menuCategoryLine() string {
	var b strings.Builder
	b.WriteString("Menu: ")
	for i, name := range menuCategoryLabels {
		if i > 0 {
			b.WriteString("  ")
		}
		if menuCategory(i) == t.menuCat {
			b.WriteByte('[')
			b.WriteString(name)
			b.WriteByte(']')
		} else {
			b.WriteString(name)
		}
	}
	b.WriteString("  (Esc)")
	return b.String()
}

func (t *Task) menuItemLine(it menuItem) string {
	switch it.id {
	case menuItemToggleScan:
		if t.scanActive {
			return it.label + "  [ON]"
		}
		return it.label + "  [OFF]"
	case menuItemToggleWaterfall:
		if t.waterfallFrozen {
			return it.label + "  [FROZEN]"
		}
		return it.label + "  [RUN]"
	case menuItemToggleCapture:
		if t.capturePaused {
			return it.label + "  [PAUSED]"
		}
		return it.label + "  [LIVE]"
	case menuItemCycleRate:
		return it.label + "  <" + t.dataRate.String() + ">"
	case menuItemCycleCRC:
		return it.label + "  <" + t.crcMode.String() + ">"
	case menuItemToggleAutoAck:
		if t.autoAck {
			return it.label + "  [x]"
		}
		return it.label + "  [ ]"
	case menuItemCyclePower:
		return it.label + "  <" + t.powerLevel.String() + ">"
	case menuItemCyclePalette:
		return it.label + "  <" + t.wfPalette.String() + ">"
	case menuItemToggleProtoMode:
		return it.label + "  <" + t.protoMode.String() + ">"
	default:
		return it.label
	}
}

func (t *Task) renderPromptOverlay(l layout) {
	boxCols := t.cols - 6
	if boxCols > 54 {
		boxCols = 54
	}
	boxRows := 10
	px := int16(3) * t.fontWidth
	py := int16(headerRows+3) * t.fontHeight
	pw := int16(boxCols) * t.fontWidth
	ph := int16(boxRows) * t.fontHeight

	_ = t.d.FillRectangle(px, py, pw, ph, colorBorder)
	_ = t.d.FillRectangle(px+1, py+1, pw-2, ph-2, colorPanelBG)
	_ = t.d.FillRectangle(px+1, py+1, pw-2, t.fontHeight+1, colorHeaderBG)

	title := t.promptTitle
	if title == "" {
		title = "Input"
	}
	t.drawStringClipped(px+2, py, title+"  (Enter apply, Esc cancel)", colorFG, boxCols)

	fieldY := py + 2*t.fontHeight
	_ = t.d.FillRectangle(px+2, fieldY, pw-4, t.fontHeight+2, colorHeaderBG)

	text := string(t.promptBuf)
	t.drawStringClipped(px+4, fieldY+1, text, colorFG, boxCols-2)

	// Cursor
	cx := px + 4 + int16(t.promptCursor)*t.fontWidth
	_ = t.d.FillRectangle(cx, fieldY+1, 1, t.fontHeight, colorAccent)

	if t.promptErr != "" {
		errY := fieldY + 2*t.fontHeight
		t.drawStringClipped(px+2, errY, "ERR: "+t.promptErr, colorWarn, boxCols)
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
	t.drawStringClipped(px+2, py, "Filters (Esc/f close)", colorFG, boxCols)
	t.drawStringClipped(px+2, py+t.fontHeight, "Up/Down sel  Left/Right adj  Enter addr", colorDim, boxCols)

	lines := make([]string, 0, filterLines)
	lines = append(lines, fmt.Sprintf("CRC    <%s>", t.filterCRC))

	chLine := fmt.Sprintf("CH     <%s>", t.filterChannel)
	switch t.filterChannel {
	case filterChannelSelected:
		chLine += fmt.Sprintf("  ch=%03d", t.selectedChannel)
	case filterChannelRange:
		chLine += fmt.Sprintf("  %03d-%03d", t.channelRangeLo, t.channelRangeHi)
	}
	lines = append(lines, chLine)

	lines = append(lines, fmt.Sprintf("MINLEN [%02d]", clampInt(t.filterMinLen, 0, 32)))
	maxText := "--"
	if t.filterMaxLen > 0 {
		maxText = fmt.Sprintf("%02d", clampInt(t.filterMaxLen, 0, 32))
	}
	lines = append(lines, fmt.Sprintf("MAXLEN [%s]", maxText))

	addr := "(none)"
	if t.filterAddrLen > 0 {
		addr = t.filterAddrHex()
	}
	lines = append(lines, fmt.Sprintf("ADDR   <%s>", addr))

	y0 := py + 2*t.fontHeight
	for i := 0; i < len(lines); i++ {
		y := y0 + int16(i)*t.fontHeight
		fg := colorFG
		bg := colorPanelBG
		if i == t.filterSel {
			fg = colorSelFG
			bg = colorSelBG
		}
		_ = t.d.FillRectangle(px+1, y, pw-2, t.fontHeight, bg)
		t.drawStringClipped(px+2, y, lines[i], fg, boxCols)
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

func sliderASCII(value, min, max, width int) string {
	if width <= 0 {
		return "[]"
	}
	if max <= min {
		var b strings.Builder
		b.Grow(width + 2)
		b.WriteByte('[')
		for i := 0; i < width; i++ {
			b.WriteByte('-')
		}
		b.WriteByte(']')
		return b.String()
	}

	value = clampInt(value, min, max)
	fill := (value - min) * width / (max - min)
	if fill < 0 {
		fill = 0
	}
	if fill > width {
		fill = width
	}

	var b strings.Builder
	b.Grow(width + 2)
	b.WriteByte('[')
	for i := 0; i < width; i++ {
		if i < fill {
			b.WriteByte('#')
		} else {
			b.WriteByte('-')
		}
	}
	b.WriteByte(']')
	return b.String()
}

func checkboxASCII(on bool) string {
	if on {
		return "[x]"
	}
	return "[ ]"
}

func (t *Task) blitWaterfall(plot rect) {
	if t.fb == nil || t.fb.Format() != hal.PixelFormatRGB565 {
		return
	}
	buf := t.fb.Buffer()
	if buf == nil {
		return
	}
	if t.wfBuf == nil || t.wfW <= 0 || t.wfH <= 0 {
		return
	}

	fbW := t.fb.Width()
	fbH := t.fb.Height()
	stride := t.fb.StrideBytes()

	px0 := int(plot.x)
	py0 := int(plot.y)
	pw := int(plot.w)
	ph := int(plot.h)
	if pw <= 0 || ph <= 0 {
		return
	}
	if px0 < 0 || py0 < 0 || px0+pw > fbW || py0+ph > fbH {
		return
	}

	for y := 0; y < ph; y++ {
		row := t.wfHead - 1 - y
		for row < 0 {
			row += t.wfH
		}
		if row >= t.wfH {
			row %= t.wfH
		}

		srcBase := row * t.wfW
		dstOff := (py0+y)*stride + px0*2
		for x := 0; x < pw; x++ {
			level := t.wfBuf[srcBase+x]
			pixel := t.wfPalette565[level]
			off := dstOff + x*2
			if off < 0 || off+1 >= len(buf) {
				continue
			}
			buf[off] = byte(pixel)
			buf[off+1] = byte(pixel >> 8)
		}
	}
}
