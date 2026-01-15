package rfanalyzer

import (
	"fmt"

	"spark/sparkos/kernel"
)

const filterLines = 5

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
		if t.filterSel == 4 {
			initial := t.filterAddrHex()
			t.openPrompt(promptSetFilterAddr, "Filter address prefix (hex, empty clears)", initial)
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
	return s
}

func (t *Task) filterAddrHex() string {
	if t.filterAddrLen <= 0 {
		return ""
	}
	out := ""
	for i := 0; i < t.filterAddrLen && i < len(t.filterAddr); i++ {
		out += fmt.Sprintf("%02X", t.filterAddr[i])
	}
	return out
}
