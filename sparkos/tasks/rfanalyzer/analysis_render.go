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
	y0 := inner.y + t.fontHeight + 1
	t.drawStringClipped(inner.x+2, y0, tabs, colorDim, maxCols)

	y := y0 + t.fontHeight
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
	case analysisCorrelation:
		t.renderAnalysisCorrelation(inner, y, maxCols)
	case analysisComparison:
		t.renderAnalysisComparison(inner, y, maxCols)
	case analysisMonitoring:
		t.renderAnalysisMonitoring(inner, y, maxCols)
	case analysisAnnotations:
		t.renderAnalysisAnnotations(inner, y, maxCols)
	case analysisDiagnostics:
		t.renderAnalysisDiagnostics(inner, y, maxCols)
	case analysisStress:
		t.renderAnalysisStress(inner, y, maxCols)
	}
}

func (t *Task) analysisTabsLine() string {
	labels := []analysisView{analysisChannels, analysisDevices, analysisTiming, analysisCollisions, analysisCorrelation, analysisComparison, analysisMonitoring, analysisAnnotations, analysisDiagnostics, analysisStress}
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

func (t *Task) renderAnalysisCorrelation(box rect, y int16, maxCols int) {
	if t.occHistCount == 0 {
		t.drawStringClipped(box.x+2, y, "(need sweep history)", colorDim, maxCols)
		return
	}
	ref := clampInt(t.selectedChannel, 0, maxChannel)
	t.drawStringClipped(box.x+2, y, fmt.Sprintf("ref ch:%03d  win:%d sweeps", ref, t.occHistCount), colorDim, maxCols)
	y += t.fontHeight

	top := t.topCorrelatedChannels(ref, 8)
	if len(top) == 0 {
		t.drawStringClipped(box.x+2, y, "(no correlations)", colorDim, maxCols)
		return
	}
	for i, it := range top {
		line := fmt.Sprintf("ch:%03d  jacc:%2d%%  both:%d", it.ch, it.jaccPct, it.both)
		t.drawStringClipped(box.x+2, y+int16(i)*t.fontHeight, line, colorFG, maxCols)
	}

	// Hop sequence for selected packet device.
	yy := y + int16(len(top))*t.fontHeight + t.fontHeight
	if yy+t.fontHeight <= box.y+box.h {
		if d := t.deviceForSelectedPacket(); d != nil {
			t.drawStringClipped(box.x+2, yy, "hop seq: "+t.deviceHopSeqText(d, 12), colorDim, maxCols)
		}
	}
}

func (t *Task) renderAnalysisComparison(box rect, y int16, maxCols int) {
	if t.compare == nil {
		t.drawStringClipped(box.x+2, y, "(load compare session in Capture menu)", colorDim, maxCols)
		return
	}
	if t.compareErr != "" {
		t.drawStringClipped(box.x+2, y, "ERR: "+t.compareErr, colorWarn, maxCols)
		return
	}
	curName := "LIVE"
	if t.replay != nil && t.replayActive {
		curName = t.replay.name
	}
	hdr := fmt.Sprintf("cur:%s  cmp:%s", curName, t.compare.name)
	t.drawStringClipped(box.x+2, y, hdr, colorDim, maxCols)
	y += t.fontHeight

	lines := t.compareChannelDiffLines(7)
	if len(lines) == 0 {
		t.drawStringClipped(box.x+2, y, "(no diff)", colorDim, maxCols)
		return
	}
	for i, line := range lines {
		t.drawStringClipped(box.x+2, y+int16(i)*t.fontHeight, line, colorFG, maxCols)
	}
}

func (t *Task) renderAnalysisMonitoring(box rect, y int16, maxCols int) {
	if !t.replayActive || t.replay == nil {
		t.drawStringClipped(box.x+2, y, "(monitoring graph: load session in replay)", colorDim, maxCols)
		return
	}
	if t.replay.bucketMs == 0 || len(t.replay.bandOccPct) == 0 {
		t.drawStringClipped(box.x+2, y, "(no long-term sweep stats)", colorDim, maxCols)
		return
	}

	bucketMs := uint64(t.replay.bucketMs)
	relNow := uint64(0)
	if t.replayNowTick >= t.replay.startTick {
		relNow = t.replayNowTick - t.replay.startTick
	}
	cur := int(relNow / bucketMs)
	if cur < 0 {
		cur = 0
	}
	if cur >= len(t.replay.bandOccPct) {
		cur = len(t.replay.bandOccPct) - 1
	}

	hdr := fmt.Sprintf("band occ: %db @%ds  cur:%d", len(t.replay.bandOccPct), bucketMs/1000, cur)
	t.drawStringClipped(box.x+2, y, hdr, colorDim, maxCols)
	y += t.fontHeight

	const win = 8
	start := cur - win/2
	if start < 0 {
		start = 0
	}
	end := start + win - 1
	if end >= len(t.replay.bandOccPct) {
		end = len(t.replay.bandOccPct) - 1
		start = end - (win - 1)
		if start < 0 {
			start = 0
		}
	}

	rows := end - start + 1
	if rows <= 0 {
		return
	}
	if t.analysisSel < 0 {
		t.analysisSel = 0
	}
	if t.analysisSel >= rows {
		t.analysisSel = rows - 1
	}

	barW := maxCols - 16
	if barW < 6 {
		barW = 6
	}

	for i := 0; i < rows; i++ {
		idx := start + i
		pct := int(t.replay.bandOccPct[idx])
		if pct < 0 {
			pct = 0
		}
		if pct > 100 {
			pct = 100
		}
		filled := pct * barW / 100
		if filled > barW {
			filled = barW
		}
		bar := make([]byte, 0, barW)
		for j := 0; j < barW; j++ {
			if j < filled {
				bar = append(bar, '#')
			} else {
				bar = append(bar, '.')
			}
		}
		line := fmt.Sprintf("m:%04d %3d%% %s", idx, pct, string(bar))

		fg := colorFG
		bg := colorPanelBG
		if idx == cur {
			fg = colorAccent
		}
		if i == t.analysisSel && t.focus == focusAnalysis {
			fg = colorSelFG
			bg = colorSelBG
		}
		_ = t.d.FillRectangle(box.x+1, y+int16(i)*t.fontHeight, box.w-2, t.fontHeight, bg)
		t.drawStringClipped(box.x+2, y+int16(i)*t.fontHeight, line, fg, maxCols)
	}
}

func (t *Task) renderAnalysisAnnotations(box rect, y int16, maxCols int) {
	now := t.nowTick
	base := uint64(0)
	total := t.annotCount
	if t.replayActive {
		now = t.replayNowTick
		if t.replay != nil {
			base = t.replay.startTick
			total = len(t.replay.annotations)
		} else {
			total = 0
		}
	}

	hdr := fmt.Sprintf("annotations:%d  (Capture menu)", total)
	if t.replayActive {
		hdr = fmt.Sprintf("annotations:%d  (Enter jump)", total)
	}
	t.drawStringClipped(box.x+2, y, hdr, colorDim, maxCols)
	y += t.fontHeight

	notes := t.visibleAnnotations(now, 8)
	if len(notes) == 0 {
		t.drawStringClipped(box.x+2, y, "(none)", colorDim, maxCols)
		return
	}
	if t.analysisSel < 0 {
		t.analysisSel = 0
	}
	if t.analysisSel >= len(notes) {
		t.analysisSel = len(notes) - 1
	}

	for i, a := range notes {
		ts := int64(a.startTick)
		if base != 0 && a.startTick >= base {
			ts = int64(a.startTick - base)
		}
		dur := a.durationMs()
		line := fmt.Sprintf("t:%06d +%04d  %s %s", ts, dur, a.tag, a.note)

		fg := colorFG
		bg := colorPanelBG
		active := a.startTick != 0 && now >= a.startTick && now <= a.endTick && a.endTick > a.startTick
		if active {
			fg = colorAccent
		}
		if i == t.analysisSel && t.focus == focusAnalysis {
			fg = colorSelFG
			bg = colorSelBG
		}
		_ = t.d.FillRectangle(box.x+1, y+int16(i)*t.fontHeight, box.w-2, t.fontHeight, bg)
		t.drawStringClipped(box.x+2, y+int16(i)*t.fontHeight, line, fg, maxCols)
	}
}

func (t *Task) renderAnalysisDiagnostics(box rect, y int16, maxCols int) {
	age := "never"
	if t.diagLastRunTick != 0 && t.nowTick > t.diagLastRunTick {
		age = fmt.Sprintf("%ds ago", int((t.nowTick-t.diagLastRunTick)/1000))
	}
	t.drawStringClipped(box.x+2, y, "Enter: run diagnostics  last: "+age, colorDim, maxCols)
	y += t.fontHeight

	rf := "FAIL"
	if t.diagRFOK {
		rf = "OK"
	}
	spi := "FAIL"
	if t.diagSPIOK {
		spi = "OK"
	}
	tim := "FAIL"
	if t.diagTimingOK {
		tim = "OK"
	}
	stab := t.diagStabilityScore

	lines := []string{
		fmt.Sprintf("RF      : %s", rf),
		fmt.Sprintf("SPI     : %s", spi),
		fmt.Sprintf("TICK    : %s  avg:%d min:%d max:%d", tim, t.tickStatsAvgMs, t.tickStatsMinMs, t.tickStatsMaxMs),
		fmt.Sprintf("STAB    : %d/100  drop:%d recerr:%v", stab, t.pktDropped, t.recordErr != ""),
	}
	for i, line := range lines {
		if y+int16(i)*t.fontHeight+t.fontHeight > box.y+box.h {
			return
		}
		t.drawStringClipped(box.x+2, y+int16(i)*t.fontHeight, line, colorFG, maxCols)
	}
}

func (t *Task) renderAnalysisStress(box rect, y int16, maxCols int) {
	ch := clampInt(t.selectedChannel, 0, maxChannel)
	hdr := fmt.Sprintf("stress: ch:%03d rate:%s", ch, t.dataRate.String())
	t.drawStringClipped(box.x+2, y, hdr, colorDim, maxCols)
	y += t.fontHeight

	run := "OFF"
	if t.stressRunning {
		run = "ON"
	}
	dur := "manual"
	if t.stressDurationMs > 0 {
		dur = fmt.Sprintf("%dms", t.stressDurationMs)
	}
	lossPct := 0
	if t.stressSent > 0 {
		lossPct = int(t.stressLost * 100 / t.stressSent)
	}
	stats := fmt.Sprintf("sent:%d rx:%d lost:%d (%d%%) lat:%d/%dms", t.stressSent, t.stressRecv, t.stressLost, lossPct, t.stressLatAvgMs, t.stressLatMaxMs)

	lines := []string{
		fmt.Sprintf("RUN     <%s>", run),
		fmt.Sprintf("PPS     [%d]", clampInt(t.stressPPS, 1, 1000)),
		fmt.Sprintf("DURms   [%s]", dur),
		stats,
	}
	if t.analysisSel < 0 {
		t.analysisSel = 0
	}
	if t.analysisSel >= len(lines) {
		t.analysisSel = len(lines) - 1
	}

	for i := 0; i < len(lines); i++ {
		yy := y + int16(i)*t.fontHeight
		fg := colorFG
		bg := colorPanelBG
		if i == t.analysisSel && t.focus == focusAnalysis {
			fg = colorSelFG
			bg = colorSelBG
		}
		_ = t.d.FillRectangle(box.x+1, yy, box.w-2, t.fontHeight, bg)
		t.drawStringClipped(box.x+2, yy, lines[i], fg, maxCols)
	}
}
