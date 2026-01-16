package rfanalyzer

import "fmt"

func (t *Task) renderAutomationOverlay(l layout) {
	boxCols := t.cols - 10
	if boxCols > 64 {
		boxCols = 64
	}
	if boxCols < 30 {
		boxCols = 30
	}
	boxRows := 3 + automationLines
	px := int16(5) * t.fontWidth
	py := int16(headerRows+3) * t.fontHeight
	pw := int16(boxCols) * t.fontWidth
	ph := int16(boxRows) * t.fontHeight

	_ = t.d.FillRectangle(px, py, pw, ph, colorBorder)
	_ = t.d.FillRectangle(px+1, py+1, pw-2, ph-2, colorPanelBG)
	_ = t.d.FillRectangle(px+1, py+1, pw-2, t.fontHeight+1, colorHeaderBG)
	t.drawStringClipped(px+2, py+1, "Automation (Esc close)", colorFG, boxCols)

	status := t.automationStatusLine(t.nowTick)
	t.drawStringClipped(px+2, py+t.fontHeight+2, status, colorDim, boxCols)

	lines := make([]string, 0, automationLines)

	arm := "OFF"
	if t.autoArmed {
		if t.autoStarted {
			arm = "RUN"
		} else {
			arm = "ARM"
		}
	}
	lines = append(lines, fmt.Sprintf("ARM     <%s>", arm))
	lines = append(lines, fmt.Sprintf("START+ms [%d]", clampInt(t.autoStartDelayMs, 0, 1_000_000)))

	dur := "off"
	if t.autoDurationMs > 0 {
		dur = fmt.Sprintf("%d", clampInt(t.autoDurationMs, 0, 1_000_000))
	}
	lines = append(lines, fmt.Sprintf("DURms   [%s]", dur))

	sw := "off"
	if t.autoStopSweeps > 0 {
		sw = fmt.Sprintf("%d", clampInt(t.autoStopSweeps, 0, 1_000_000))
	}
	lines = append(lines, fmt.Sprintf("STOP_SW [%s]", sw))

	pk := "off"
	if t.autoStopPackets > 0 {
		pk = fmt.Sprintf("%d", clampInt(t.autoStopPackets, 0, 1_000_000))
	}
	lines = append(lines, fmt.Sprintf("STOP_PK [%s]", pk))

	rec := "OFF"
	if t.autoRecord {
		rec = "ON"
	}
	lines = append(lines, fmt.Sprintf("RECORD  <%s>", rec))

	name := t.autoSessionBase
	if name == "" {
		name = "(auto)"
	}
	lines = append(lines, fmt.Sprintf("NAME    <%s>", name))

	y0 := py + 2*t.fontHeight + 2
	for i := 0; i < len(lines); i++ {
		y := y0 + int16(i)*t.fontHeight
		fg := colorFG
		bg := colorPanelBG
		if i == t.autoSel {
			fg = colorSelFG
			bg = colorSelBG
		}
		_ = t.d.FillRectangle(px+1, y, pw-2, t.fontHeight, bg)
		t.drawStringClipped(px+2, y, lines[i], fg, boxCols)
	}

	hint := "Up/Down sel  Left/Right adj  Enter edit/toggle"
	t.drawStringClipped(px+2, py+ph-t.fontHeight-1, hint, colorDim, boxCols)
}
