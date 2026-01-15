package rfanalyzer

import (
	"errors"
	"fmt"
	"strings"

	"spark/sparkos/kernel"
)

type promptKind uint8

const (
	promptSetChannel promptKind = iota
	promptSetRangeLo
	promptSetRangeHi
	promptSetDwell
	promptSetScanStep
	promptSavePreset
	promptLoadPreset
)

func (t *Task) openPrompt(kind promptKind, title, initial string) {
	t.showPrompt = true
	t.promptKind = kind
	t.promptTitle = title
	t.promptErr = ""
	t.promptBuf = []rune(initial)
	t.promptCursor = len(t.promptBuf)
	t.invalidate(dirtyOverlay | dirtyStatus)
}

func (t *Task) closePrompt() {
	if !t.showPrompt {
		return
	}
	t.showPrompt = false
	t.promptErr = ""
	t.promptTitle = ""
	t.promptBuf = nil
	t.promptCursor = 0
	t.invalidate(dirtyOverlay | dirtyStatus)
}

func (t *Task) handlePromptKey(ctx *kernel.Context, k key) {
	switch k.kind {
	case keyEsc:
		t.closePrompt()
		return
	case keyEnter:
		t.submitPrompt(ctx)
		return
	case keyLeft:
		if t.promptCursor > 0 {
			t.promptCursor--
			t.invalidate(dirtyOverlay)
		}
		return
	case keyRight:
		if t.promptCursor < len(t.promptBuf) {
			t.promptCursor++
			t.invalidate(dirtyOverlay)
		}
		return
	case keyBackspace:
		if t.promptCursor > 0 && len(t.promptBuf) > 0 {
			t.promptBuf = append(t.promptBuf[:t.promptCursor-1], t.promptBuf[t.promptCursor:]...)
			t.promptCursor--
			t.invalidate(dirtyOverlay)
		}
		return
	case keyDelete:
		if t.promptCursor >= 0 && t.promptCursor < len(t.promptBuf) {
			t.promptBuf = append(t.promptBuf[:t.promptCursor], t.promptBuf[t.promptCursor+1:]...)
			t.invalidate(dirtyOverlay)
		}
		return
	case keyRune:
		t.insertPromptRune(k.r)
		return
	default:
		return
	}
}

func (t *Task) insertPromptRune(r rune) {
	if r == 0 {
		return
	}
	if r == '\n' || r == '\r' || r == '\t' {
		return
	}

	if isNumericPrompt(t.promptKind) {
		if r < '0' || r > '9' {
			return
		}
	} else {
		// preset names: printable ASCII subset.
		if r < 0x20 || r > 0x7e {
			return
		}
	}

	if len(t.promptBuf) >= 32 {
		return
	}
	t.promptBuf = append(t.promptBuf, 0)
	copy(t.promptBuf[t.promptCursor+1:], t.promptBuf[t.promptCursor:])
	t.promptBuf[t.promptCursor] = r
	t.promptCursor++
	t.invalidate(dirtyOverlay)
}

func isNumericPrompt(k promptKind) bool {
	switch k {
	case promptSetChannel, promptSetRangeLo, promptSetRangeHi, promptSetDwell, promptSetScanStep:
		return true
	default:
		return false
	}
}

func (t *Task) submitPrompt(ctx *kernel.Context) {
	s := strings.TrimSpace(string(t.promptBuf))
	switch t.promptKind {
	case promptSetChannel:
		n, err := parseIntStrict(s)
		if err != nil {
			t.promptErr = "channel: " + err.Error()
			t.invalidate(dirtyOverlay)
			return
		}
		if n < 0 || n > maxChannel {
			t.promptErr = fmt.Sprintf("channel: must be 0..%d", maxChannel)
			t.invalidate(dirtyOverlay)
			return
		}
		t.selectedChannel = n
		t.closePrompt()
		t.invalidate(dirtySpectrum | dirtyWaterfall | dirtyStatus)
		return

	case promptSetRangeLo:
		n, err := parseIntStrict(s)
		if err != nil {
			t.promptErr = "range lo: " + err.Error()
			t.invalidate(dirtyOverlay)
			return
		}
		t.channelRangeLo = clampInt(n, 0, maxChannel)
		if t.channelRangeLo > t.channelRangeHi {
			t.channelRangeHi = t.channelRangeLo
		}
		t.presetDirty = true
		t.scanNextTick = 0
		t.closePrompt()
		t.invalidate(dirtyRFControl | dirtySpectrum | dirtyWaterfall | dirtyStatus)
		return

	case promptSetRangeHi:
		n, err := parseIntStrict(s)
		if err != nil {
			t.promptErr = "range hi: " + err.Error()
			t.invalidate(dirtyOverlay)
			return
		}
		t.channelRangeHi = clampInt(n, 0, maxChannel)
		if t.channelRangeHi < t.channelRangeLo {
			t.channelRangeLo = t.channelRangeHi
		}
		t.presetDirty = true
		t.scanNextTick = 0
		t.closePrompt()
		t.invalidate(dirtyRFControl | dirtySpectrum | dirtyWaterfall | dirtyStatus)
		return

	case promptSetDwell:
		n, err := parseIntStrict(s)
		if err != nil {
			t.promptErr = "dwell: " + err.Error()
			t.invalidate(dirtyOverlay)
			return
		}
		t.dwellTimeMs = clampInt(n, 1, 50)
		t.presetDirty = true
		t.scanNextTick = 0
		t.closePrompt()
		t.invalidate(dirtyRFControl | dirtySpectrum | dirtyStatus)
		return

	case promptSetScanStep:
		n, err := parseIntStrict(s)
		if err != nil {
			t.promptErr = "scan step: " + err.Error()
			t.invalidate(dirtyOverlay)
			return
		}
		t.scanSpeedScalar = clampInt(n, 1, 10)
		t.presetDirty = true
		t.scanNextTick = 0
		t.closePrompt()
		t.invalidate(dirtyRFControl | dirtySpectrum | dirtyStatus)
		return

	case promptSavePreset:
		if err := t.savePreset(ctx, s); err != nil {
			t.promptErr = err.Error()
			t.invalidate(dirtyOverlay)
			return
		}
		t.closePrompt()
		t.invalidate(dirtyRFControl | dirtyStatus)
		return

	case promptLoadPreset:
		if err := t.loadPreset(ctx, s); err != nil {
			t.promptErr = err.Error()
			t.invalidate(dirtyOverlay)
			return
		}
		t.closePrompt()
		return

	default:
		t.promptErr = "unsupported prompt"
		t.invalidate(dirtyOverlay)
		return
	}
}

func parseIntStrict(s string) (int, error) {
	if s == "" {
		return 0, errors.New("empty")
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, errors.New("bad number")
		}
		n = n*10 + int(r-'0')
		if n > 1_000_000 {
			return 0, errors.New("too large")
		}
	}
	return n, nil
}
