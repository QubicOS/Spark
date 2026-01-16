package rfanalyzer

import (
	"fmt"

	"spark/sparkos/kernel"
)

const filterLines = 8

func (t *Task) handleFiltersKey(ctx *kernel.Context, k key) {
	switch k.kind {
	case keyEsc:
		t.showFilters = false
		t.invalidate(dirtyOverlay | dirtySniffer | dirtyStatus)
		return
	case keyRune:
		if k.r == 'f' || k.r == 'F' {
			t.showFilters = false
			t.invalidate(dirtyOverlay | dirtySniffer | dirtyStatus)
			return
		}
	case keyUp:
		t.filterSel--
		if t.filterSel < 0 {
			t.filterSel = filterLines - 1
		}
		t.invalidate(dirtyOverlay)
		return
	case keyDown:
		t.filterSel++
		if t.filterSel >= filterLines {
			t.filterSel = 0
		}
		t.invalidate(dirtyOverlay)
		return
	case keyLeft:
		t.adjustFilter(ctx, -1)
		return
	case keyRight:
		t.adjustFilter(ctx, +1)
		return
	case keyEnter:
		switch t.filterSel {
		case 4:
			initial := t.filterAddrHex()
			t.openPrompt(promptSetFilterAddr, "Filter address mask (hex, use ??, empty clears)", initial)
			t.showFilters = false
		case 5:
			initial := t.filterPayloadHex()
			t.openPrompt(promptSetFilterPayload, "Filter payload prefix (hex, use ??, empty clears)", initial)
			t.showFilters = false
		case 6:
			t.openPrompt(promptSetFilterAge, "Filter age window (ms, 0 disables)", fmt.Sprintf("%d", t.filterAgeMs))
			t.showFilters = false
		case 7:
			t.openPrompt(promptSetFilterBurst, "Filter burst Δt max (ms, 0 disables)", fmt.Sprintf("%d", t.filterBurstMaxMs))
			t.showFilters = false
		}
		return
	}
}

func (t *Task) adjustFilter(_ *kernel.Context, delta int) {
	changed := false
	switch t.filterSel {
	case 0: // CRC
		t.filterCRC = filterCRC(wrapEnum(int(t.filterCRC)+delta, 3))
		changed = true
	case 1: // CH mode
		t.filterChannel = filterChannel(wrapEnum(int(t.filterChannel)+delta, 3))
		changed = true
	case 2: // MINLEN
		t.filterMinLen = clampInt(t.filterMinLen+delta, 0, 32)
		if t.filterMaxLen > 0 && t.filterMinLen > t.filterMaxLen {
			t.filterMaxLen = t.filterMinLen
		}
		changed = true
	case 3: // MAXLEN
		t.filterMaxLen = clampInt(t.filterMaxLen+delta, 0, 32)
		if t.filterMaxLen > 0 && t.filterMaxLen < t.filterMinLen {
			t.filterMinLen = t.filterMaxLen
		}
		changed = true
	case 6: // AGEms
		t.filterAgeMs = clampInt(t.filterAgeMs+delta*100, 0, 1_000_000)
		changed = true
	case 7: // BURSTΔ
		t.filterBurstMaxMs = clampInt(t.filterBurstMaxMs+delta, 0, 1_000_000)
		changed = true
	}
	if !changed {
		return
	}
	t.reconcileSnifferSelection()
	t.invalidate(dirtyOverlay | dirtySniffer | dirtyProtocol | dirtyStatus)
}

func (t *Task) filterSummary() string {
	s := fmt.Sprintf("CRC:%s CH:%s", t.filterCRC, t.filterChannel)
	if t.filterMinLen > 0 || t.filterMaxLen > 0 {
		s += fmt.Sprintf(" LEN:%d..%d", t.filterMinLen, t.filterMaxLen)
	}
	if t.filterAddrLen > 0 {
		s += " ADDR:" + t.filterAddrHex()
	}
	if t.filterPayloadLen > 0 {
		s += " PAY:" + t.filterPayloadHex()
	}
	if t.filterAgeMs > 0 {
		s += fmt.Sprintf(" AGE:%dms", t.filterAgeMs)
	}
	if t.filterBurstMaxMs > 0 {
		s += fmt.Sprintf(" BΔ<=%dms", t.filterBurstMaxMs)
	}
	return s
}

func (t *Task) filterAddrHex() string {
	if t.filterAddrLen <= 0 {
		return ""
	}
	out := ""
	for i := 0; i < t.filterAddrLen && i < len(t.filterAddr); i++ {
		out += hexMaskByte(t.filterAddr[i], t.filterAddrMask[i])
	}
	return out
}

func (t *Task) filterPayloadHex() string {
	if t.filterPayloadLen <= 0 {
		return ""
	}
	out := ""
	for i := 0; i < t.filterPayloadLen && i < len(t.filterPayload); i++ {
		out += hexMaskByte(t.filterPayload[i], t.filterPayloadMask[i])
	}
	return out
}

func hexMaskByte(v, mask byte) string {
	hi := byte('?')
	lo := byte('?')
	if (mask & 0xF0) != 0 {
		hi = hexDigit(v >> 4)
	}
	if (mask & 0x0F) != 0 {
		lo = hexDigit(v & 0x0F)
	}
	return string([]byte{hi, lo})
}

func hexDigit(v byte) byte {
	v &= 0x0F
	if v < 10 {
		return '0' + v
	}
	return 'A' + (v - 10)
}
