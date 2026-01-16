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
	promptSetFilterPayload
	promptSetFilterAge
	promptSetFilterBurst
	promptStartRecording
	promptLoadSession
	promptLoadCompareSession
	promptReplaySeek
	promptExportCSV
	promptExportPCAP
	promptExportRFPKT
	promptAutoName
	promptAutoStartDelay
	promptAutoDuration
	promptAutoStopSweeps
	promptAutoStopPackets
	promptStressPPS
	promptStressDuration
	promptAnnotTag
	promptAnnotNote
	promptAnnotDuration
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
		if t.promptKind == promptAnnotTag || t.promptKind == promptAnnotNote || t.promptKind == promptAnnotDuration {
			t.annotPending = annotation{}
		}
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
	case promptSetChannel, promptSetRangeLo, promptSetRangeHi, promptSetDwell, promptSetScanStep, promptSetFilterAge, promptSetFilterBurst, promptReplaySeek, promptAutoStartDelay, promptAutoDuration, promptAutoStopSweeps, promptAutoStopPackets, promptStressPPS, promptStressDuration, promptAnnotDuration:
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
		val, mask, err := parseHexMaskPattern(s, len(t.filterAddr))
		if err != nil {
			t.promptErr = "addr: " + err.Error()
			t.invalidate(dirtyOverlay)
			return
		}
		t.filterAddrLen = len(val)
		for i := 0; i < t.filterAddrLen; i++ {
			t.filterAddr[i] = val[i]
			t.filterAddrMask[i] = mask[i]
		}
		for i := t.filterAddrLen; i < len(t.filterAddr); i++ {
			t.filterAddr[i] = 0
			t.filterAddrMask[i] = 0
		}
		t.reconcileSnifferSelection()
		t.closePrompt()
		t.invalidate(dirtySniffer | dirtyProtocol | dirtyStatus)
		return

	case promptSetFilterPayload:
		s = strings.ReplaceAll(strings.TrimSpace(s), " ", "")
		if s == "" {
			t.filterPayloadLen = 0
			for i := range t.filterPayload {
				t.filterPayload[i] = 0
				t.filterPayloadMask[i] = 0
			}
			t.reconcileSnifferSelection()
			t.closePrompt()
			t.invalidate(dirtySniffer | dirtyProtocol | dirtyStatus)
			return
		}
		val, mask, err := parseHexMaskPattern(s, len(t.filterPayload))
		if err != nil {
			t.promptErr = "payload: " + err.Error()
			t.invalidate(dirtyOverlay)
			return
		}
		t.filterPayloadLen = len(val)
		for i := 0; i < t.filterPayloadLen; i++ {
			t.filterPayload[i] = val[i]
			t.filterPayloadMask[i] = mask[i]
		}
		for i := t.filterPayloadLen; i < len(t.filterPayload); i++ {
			t.filterPayload[i] = 0
			t.filterPayloadMask[i] = 0
		}
		t.reconcileSnifferSelection()
		t.closePrompt()
		t.invalidate(dirtySniffer | dirtyProtocol | dirtyStatus)
		return

	case promptSetFilterAge:
		n, err := parseIntStrict(s)
		if err != nil {
			t.promptErr = "age: " + err.Error()
			t.invalidate(dirtyOverlay)
			return
		}
		t.filterAgeMs = clampInt(n, 0, 1_000_000)
		t.reconcileSnifferSelection()
		t.closePrompt()
		t.invalidate(dirtySniffer | dirtyProtocol | dirtyStatus)
		return

	case promptSetFilterBurst:
		n, err := parseIntStrict(s)
		if err != nil {
			t.promptErr = "burst: " + err.Error()
			t.invalidate(dirtyOverlay)
			return
		}
		t.filterBurstMaxMs = clampInt(n, 0, 1_000_000)
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

	case promptLoadCompareSession:
		sess, err := t.loadSession(ctx, s)
		if err != nil {
			t.promptErr = err.Error()
			t.invalidate(dirtyOverlay)
			return
		}
		t.compare = sess
		t.compareErr = ""
		t.closePrompt()
		t.invalidate(dirtyAnalysis | dirtySpectrum | dirtyStatus)
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

	case promptExportCSV:
		if err := t.exportReplayCSV(ctx, s); err != nil {
			t.promptErr = "export: " + err.Error()
			t.invalidate(dirtyOverlay)
			return
		}
		t.closePrompt()
		return

	case promptExportPCAP:
		if err := t.exportReplayPCAP(ctx, s); err != nil {
			t.promptErr = "export: " + err.Error()
			t.invalidate(dirtyOverlay)
			return
		}
		t.closePrompt()
		return

	case promptExportRFPKT:
		if err := t.exportReplayRFPKT(ctx, s); err != nil {
			t.promptErr = "export: " + err.Error()
			t.invalidate(dirtyOverlay)
			return
		}
		t.closePrompt()
		return

	case promptAutoName:
		base := sanitizePresetName(s)
		if base == "" {
			t.promptErr = "name: invalid"
			t.invalidate(dirtyOverlay)
			return
		}
		t.autoSessionBase = base
		t.closePrompt()
		t.invalidate(dirtyOverlay | dirtyStatus)
		return

	case promptAutoStartDelay:
		n, err := parseIntStrict(s)
		if err != nil {
			t.promptErr = "start delay: " + err.Error()
			t.invalidate(dirtyOverlay)
			return
		}
		t.autoStartDelayMs = clampInt(n, 0, 1_000_000)
		if t.autoArmed && !t.autoStarted && ctx != nil {
			t.autoStartTick = ctx.NowTick() + uint64(t.autoStartDelayMs)
		}
		t.closePrompt()
		t.invalidate(dirtyOverlay | dirtyStatus)
		return

	case promptAutoDuration:
		n, err := parseIntStrict(s)
		if err != nil {
			t.promptErr = "duration: " + err.Error()
			t.invalidate(dirtyOverlay)
			return
		}
		t.autoDurationMs = clampInt(n, 0, 1_000_000)
		if t.autoArmed && t.autoStarted && t.autoDurationMs > 0 {
			t.autoStopTick = t.autoRunStartTick + uint64(t.autoDurationMs)
		} else if t.autoArmed && t.autoStarted && t.autoDurationMs == 0 {
			t.autoStopTick = 0
		}
		t.closePrompt()
		t.invalidate(dirtyOverlay | dirtyStatus)
		return

	case promptAutoStopSweeps:
		n, err := parseIntStrict(s)
		if err != nil {
			t.promptErr = "sweeps: " + err.Error()
			t.invalidate(dirtyOverlay)
			return
		}
		t.autoStopSweeps = clampInt(n, 0, 1_000_000)
		t.closePrompt()
		t.invalidate(dirtyOverlay | dirtyStatus)
		return

	case promptAutoStopPackets:
		n, err := parseIntStrict(s)
		if err != nil {
			t.promptErr = "packets: " + err.Error()
			t.invalidate(dirtyOverlay)
			return
		}
		t.autoStopPackets = clampInt(n, 0, 1_000_000)
		t.closePrompt()
		t.invalidate(dirtyOverlay | dirtyStatus)
		return

	case promptStressPPS:
		n, err := parseIntStrict(s)
		if err != nil {
			t.promptErr = "pps: " + err.Error()
			t.invalidate(dirtyOverlay)
			return
		}
		t.stressPPS = clampInt(n, 1, 1000)
		t.closePrompt()
		t.invalidate(dirtyOverlay | dirtyAnalysis | dirtyStatus)
		return

	case promptStressDuration:
		n, err := parseIntStrict(s)
		if err != nil {
			t.promptErr = "duration: " + err.Error()
			t.invalidate(dirtyOverlay)
			return
		}
		t.stressDurationMs = clampInt(n, 0, 1_000_000)
		t.closePrompt()
		t.invalidate(dirtyOverlay | dirtyAnalysis | dirtyStatus)
		return

	case promptAnnotTag:
		t.annotPending.tag = s
		if strings.TrimSpace(s) == "" {
			t.promptErr = "tag: empty"
			t.invalidate(dirtyOverlay)
			return
		}
		t.openPrompt(promptAnnotNote, "Annotation note", t.annotPending.note)
		return

	case promptAnnotNote:
		t.annotPending.note = s
		initial := "0"
		if t.annotLastDurMs > 0 {
			initial = fmt.Sprintf("%d", t.annotLastDurMs)
		}
		t.openPrompt(promptAnnotDuration, "Annotation duration (ms, 0=point)", initial)
		return

	case promptAnnotDuration:
		n, err := parseIntStrict(s)
		if err != nil {
			t.promptErr = "duration: " + err.Error()
			t.invalidate(dirtyOverlay)
			return
		}
		t.annotLastDurMs = clampInt(n, 0, 1_000_000)
		t.annotPending.endTick = t.annotPending.startTick + uint64(t.annotLastDurMs)
		if err := t.addAnnotation(ctx, t.annotPending); err != nil {
			t.promptErr = err.Error()
			t.invalidate(dirtyOverlay)
			return
		}
		t.annotPending = annotation{}
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

func parseHexMaskPattern(s string, maxBytes int) ([]byte, []byte, error) {
	if s == "" {
		return nil, nil, errors.New("empty")
	}
	if len(s)%2 != 0 {
		return nil, nil, errors.New("hex must be even length")
	}
	n := len(s) / 2
	if n > maxBytes {
		return nil, nil, fmt.Errorf("max %d bytes", maxBytes)
	}
	val := make([]byte, n)
	mask := make([]byte, n)
	for i := 0; i < n; i++ {
		v, m, ok := parseHexMaskByte(s[i*2 : i*2+2])
		if !ok {
			return nil, nil, errors.New("bad hex")
		}
		val[i] = v
		mask[i] = m
	}
	return val, mask, nil
}

func parseHexMaskByte(s string) (byte, byte, bool) {
	if len(s) != 2 {
		return 0, 0, false
	}

	v := byte(0)
	m := byte(0)

	if s[0] != '?' {
		hi, ok := parseHexNibble(rune(s[0]))
		if !ok {
			return 0, 0, false
		}
		v |= hi << 4
		m |= 0xF0
	}
	if s[1] != '?' {
		lo, ok := parseHexNibble(rune(s[1]))
		if !ok {
			return 0, 0, false
		}
		v |= lo
		m |= 0x0F
	}
	return v, m, true
}
