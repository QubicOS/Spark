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
	promptSetFilterAddr
	promptStartRecording
	promptLoadSession
	promptReplaySeek
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
	case promptSetChannel, promptSetRangeLo, promptSetRangeHi, promptSetDwell, promptSetScanStep, promptReplaySeek:
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
		t.recordConfig(ctx.NowTick())
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
		t.recordConfig(ctx.NowTick())
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
		t.recordConfig(ctx.NowTick())
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
		t.recordConfig(ctx.NowTick())
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
		t.recordConfig(ctx.NowTick())
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

	case promptSetFilterAddr:
		s = strings.ReplaceAll(strings.TrimSpace(s), " ", "")
		if s == "" {
			t.filterAddrLen = 0
			t.reconcileSnifferSelection()
			t.closePrompt()
			t.invalidate(dirtySniffer | dirtyProtocol | dirtyStatus)
			return
		}
		if len(s)%2 != 0 {
			t.promptErr = "addr: hex must be even length"
			t.invalidate(dirtyOverlay)
			return
		}
		if len(s)/2 > len(t.filterAddr) {
			t.promptErr = fmt.Sprintf("addr: max %d bytes", len(t.filterAddr))
			t.invalidate(dirtyOverlay)
			return
		}
		n := len(s) / 2
		for i := 0; i < n; i++ {
			b, ok := parseHexByte(s[i*2 : i*2+2])
			if !ok {
				t.promptErr = "addr: bad hex"
				t.invalidate(dirtyOverlay)
				return
			}
			t.filterAddr[i] = b
		}
		t.filterAddrLen = n
		t.reconcileSnifferSelection()
		t.closePrompt()
		t.invalidate(dirtySniffer | dirtyProtocol | dirtyStatus)
		return

	case promptStartRecording:
		if err := t.startRecording(ctx, s); err != nil {
			t.promptErr = err.Error()
			t.invalidate(dirtyOverlay)
			return
		}
		t.closePrompt()
		t.invalidate(dirtyStatus | dirtyRFControl)
		return

	case promptLoadSession:
		if err := t.enterReplay(ctx, s); err != nil {
			t.promptErr = err.Error()
			t.invalidate(dirtyOverlay)
			return
		}
		t.closePrompt()
		return

	case promptReplaySeek:
		if !t.replayActive || t.replay == nil {
			t.promptErr = "replay not active"
			t.invalidate(dirtyOverlay)
			return
		}
		n, err := parseIntStrict(s)
		if err != nil {
			t.promptErr = "seek: " + err.Error()
			t.invalidate(dirtyOverlay)
			return
		}
		newTick := t.replay.startTick + uint64(n)
		if newTick < t.replay.startTick {
			newTick = t.replay.startTick
		}
		if newTick > t.replay.endTick {
			newTick = t.replay.endTick
		}
		t.replayNowTick = newTick
		t.replayHostLastTick = 0
		t.replayPlaying = false
		t.updateReplayPosition(ctx, t.replayNowTick, true)
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

func parseHexByte(s string) (byte, bool) {
	if len(s) != 2 {
		return 0, false
	}
	hi, ok := parseHexNibble(rune(s[0]))
	if !ok {
		return 0, false
	}
	lo, ok := parseHexNibble(rune(s[1]))
	if !ok {
		return 0, false
	}
	return byte(hi<<4 | lo), true
}

func parseHexNibble(r rune) (byte, bool) {
	switch {
	case r >= '0' && r <= '9':
		return byte(r - '0'), true
	case r >= 'a' && r <= 'f':
		return byte(10 + r - 'a'), true
	case r >= 'A' && r <= 'F':
		return byte(10 + r - 'A'), true
	default:
		return 0, false
	}
}
