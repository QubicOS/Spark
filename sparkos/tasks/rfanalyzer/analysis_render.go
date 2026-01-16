package rfanalyzer

import (
	"fmt"
)

func (t *Task) renderAnalysis(l layout) {
	if l.analysis.w <= 0 || l.analysis.h <= 0 || l.analysisCols <= 0 {
		return
	}

	title := "Analysis " + t.analysisView.String()
	t.renderPanel(l.analysis, title, t.focus == focusAnalysis)

	inner := l.analysis.inset(2, 2)
	maxCols := int(inner.w / t.fontWidth)
	if maxCols <= 0 {
		return
	}

	tabs := t.analysisTabsLine()
	t.drawStringClipped(inner.x+2, inner.y, tabs, colorDim, maxCols)

	y := inner.y + t.fontHeight
	if y+t.fontHeight > inner.y+inner.h {
		return
	}

	switch t.analysisView {
	case analysisChannels:
		t.renderAnalysisChannels(inner, y, maxCols)
	case analysisDevices:
		t.renderAnalysisDevices(inner, y, maxCols)
	case analysisTiming:
		t.renderAnalysisTiming(inner, y, maxCols)
	case analysisCollisions:
		t.renderAnalysisCollisions(inner, y, maxCols)
	}
}

func (t *Task) analysisTabsLine() string {
	labels := []analysisView{analysisChannels, analysisDevices, analysisTiming, analysisCollisions}
	out := "tabs:"
	for _, v := range labels {
		if v == t.analysisView {
			out += " [" + v.String() + "]"
		} else {
			out += " " + v.String()
		}
	}
	out += "  (←/→ switch, ↑/↓ select)"
	return out
}

func (t *Task) renderAnalysisChannels(box rect, y int16, maxCols int) {
	if t.anaSweepCount == 0 {
		t.drawStringClipped(box.x+2, y, "(no sweep data yet)", colorDim, maxCols)
		return
	}
	top := t.topChannels(8)
	if len(top) == 0 {
		t.drawStringClipped(box.x+2, y, "(no channels)", colorDim, maxCols)
		return
	}

	if t.analysisSel < 0 {
		t.analysisSel = 0
	}
	if t.analysisSel >= len(top) {
		t.analysisSel = len(top) - 1
	}

	for i, cs := range top {
		occPct := int(t.anaOccCount[cs.ch] * 100 / uint32(t.anaSweepCount))
		badPct := 0
		if t.anaChanPkt[cs.ch] > 0 {
			badPct = int(t.anaChanBad[cs.ch] * 100 / t.anaChanPkt[cs.ch])
		}
		per := t.periodicText(cs.ch)
		line := fmt.Sprintf("ch:%03d  score:%3d  occ:%2d%%  bad:%2d%%  %s", cs.ch, cs.score, occPct, badPct, per)
		fg := colorFG
		bg := colorPanelBG
		if i == t.analysisSel && t.focus == focusAnalysis {
			fg = colorSelFG
			bg = colorSelBG
		}
		_ = t.d.FillRectangle(box.x+1, y+int16(i)*t.fontHeight, box.w-2, t.fontHeight, bg)
		t.drawStringClipped(box.x+2, y+int16(i)*t.fontHeight, line, fg, maxCols)
	}

	bestLine := t.bestHistoryLine(4)
	yy := y + int16(len(top))*t.fontHeight
	if yy+t.fontHeight <= box.y+box.h {
		t.drawStringClipped(box.x+2, yy, bestLine, colorDim, maxCols)
	}
}

func (t *Task) bestHistoryLine(n int) string {
	if t.bestCount == 0 {
		return "best: (none)"
	}
	if n < 1 {
		n = 1
	}
	if n > t.bestCount {
		n = t.bestCount
	}
	out := "best:"
	i := t.bestHead - 1
	for k := 0; k < n; k++ {
		if i < 0 {
			i = len(t.bestHist) - 1
		}
		ev := t.bestHist[i]
		if ev.score == 0 {
			i--
			continue
		}
		out += fmt.Sprintf(" %03d(%d)", ev.ch, ev.score)
		i--
	}
	return out
}

func (t *Task) renderAnalysisDevices(box rect, y int16, maxCols int) {
	now := t.nowTick
	if t.replayActive {
		now = t.replayNowTick
	}
	active := 0
	for i := range t.devices {
		d := &t.devices[i]
		if !d.used {
			continue
		}
		if d.lastTick != 0 && now > d.lastTick && now-d.lastTick <= 5000 {
			active++
		}
	}
	hdr := fmt.Sprintf("devices: active:%d total:%d  (by addr)", active, t.deviceCount)
	t.drawStringClipped(box.x+2, y, hdr, colorDim, maxCols)
	y += t.fontHeight

	top := t.topDeviceIndices(8)
	if len(top) == 0 {
		t.drawStringClipped(box.x+2, y, "(no devices)", colorDim, maxCols)
		return
	}
	if t.analysisSel < 0 {
		t.analysisSel = 0
	}
	if t.analysisSel >= len(top) {
		t.analysisSel = len(top) - 1
	}
	for i, idx := range top {
		d := &t.devices[idx]
		badPct := 0
		if d.crcOK+d.crcBad > 0 {
			badPct = int(d.crcBad * 100 / (d.crcOK + d.crcBad))
		}
		age := "-"
		if d.lastTick != 0 && now > d.lastTick {
			age = fmt.Sprintf("%ds", int((now-d.lastTick)/1000))
		}
		line := fmt.Sprintf("%s  pk:%4d bad:%2d%% hop:%3d rty:%3d last:%s", d.addrSuffix3(), d.pktCount, badPct, d.hopCount, d.retries, age)
		fg := colorFG
		bg := colorPanelBG
		if i == t.analysisSel && t.focus == focusAnalysis {
			fg = colorSelFG
			bg = colorSelBG
		}
		_ = t.d.FillRectangle(box.x+1, y+int16(i)*t.fontHeight, box.w-2, t.fontHeight, bg)
		t.drawStringClipped(box.x+2, y+int16(i)*t.fontHeight, line, fg, maxCols)
	}
}

func (t *Task) renderAnalysisTiming(box rect, y int16, maxCols int) {
	addrLen, addr, ok := t.selectedPacketAddr()
	if !ok {
		t.drawStringClipped(box.x+2, y, "(select a packet for device timing)", colorDim, maxCols)
		return
	}
	d := t.findDevice(addrLen, addr)
	if d == nil {
		t.drawStringClipped(box.x+2, y, "(device not tracked yet)", colorDim, maxCols)
		return
	}
	h1 := fmt.Sprintf("dev:%s  pk:%d bad:%d rty:%d", d.addrSuffix3(), d.pktCount, d.crcBad, d.retries)
	t.drawStringClipped(box.x+2, y, h1, colorFG, maxCols)
	y += t.fontHeight

	if d.intCount == 0 {
		t.drawStringClipped(box.x+2, y, "(need more packets)", colorDim, maxCols)
		return
	}
	h2 := fmt.Sprintf("int(avg:%dms jit:%dms min:%d max:%d)", d.intAvg, d.intJitter, d.intMin, d.intMax)
	h3 := fmt.Sprintf("bursts:%d maxlen:%d hop:%d", d.burstCount, d.burstMax, d.hopCount)
	t.drawStringClipped(box.x+2, y, h2, colorDim, maxCols)
	y += t.fontHeight
	t.drawStringClipped(box.x+2, y, h3, colorDim, maxCols)
	y += t.fontHeight

	slot := ""
	if d.intCount >= 6 && d.intAvg > 0 && d.intJitter <= d.intAvg/10 {
		slot = fmt.Sprintf("slot: ~%dms (low jitter)", d.intAvg)
	}
	if slot != "" && y+t.fontHeight <= box.y+box.h {
		t.drawStringClipped(box.x+2, y, slot, colorDim, maxCols)
	}
}

func (t *Task) renderAnalysisCollisions(box rect, y int16, maxCols int) {
	if t.anaChanPkt == ([numChannels]uint32{}) {
		t.drawStringClipped(box.x+2, y, "(no packet stats yet)", colorDim, maxCols)
		return
	}

	lines := t.topConflictChannels(8)
	if len(lines) == 0 {
		t.drawStringClipped(box.x+2, y, "(no conflicts)", colorDim, maxCols)
		return
	}

	for i, line := range lines {
		t.drawStringClipped(box.x+2, y+int16(i)*t.fontHeight, line, colorFG, maxCols)
	}
}
