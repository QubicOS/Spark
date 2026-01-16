package rfanalyzer

import (
	"fmt"
	"strings"

	"spark/sparkos/kernel"
)

const automationLines = 7

func (t *Task) handleAutomationKey(ctx *kernel.Context, k key) {
	switch k.kind {
	case keyEsc:
		t.showAutomation = false
		t.invalidate(dirtyOverlay | dirtyStatus)
		return
	case keyUp:
		t.autoSel--
		if t.autoSel < 0 {
			t.autoSel = automationLines - 1
		}
		t.invalidate(dirtyOverlay)
		return
	case keyDown:
		t.autoSel++
		if t.autoSel >= automationLines {
			t.autoSel = 0
		}
		t.invalidate(dirtyOverlay)
		return
	case keyLeft:
		t.adjustAutomation(ctx, -1)
		return
	case keyRight:
		t.adjustAutomation(ctx, +1)
		return
	case keyEnter:
		t.activateAutomationLine(ctx)
		return
	}
}

func (t *Task) adjustAutomation(ctx *kernel.Context, delta int) {
	changed := false
	switch t.autoSel {
	case 0: // ARM
		if delta != 0 {
			t.toggleAutomationArm(ctx)
			changed = true
		}
	case 1: // START+ms
		t.autoStartDelayMs = clampInt(t.autoStartDelayMs+delta*1000, 0, 1_000_000)
		changed = true
	case 2: // DURms
		t.autoDurationMs = clampInt(t.autoDurationMs+delta*10_000, 0, 1_000_000)
		changed = true
	case 3: // STOP_SW
		t.autoStopSweeps = clampInt(t.autoStopSweeps+delta*10, 0, 1_000_000)
		changed = true
	case 4: // STOP_PK
		t.autoStopPackets = clampInt(t.autoStopPackets+delta*10, 0, 1_000_000)
		changed = true
	case 5: // REC
		if delta != 0 {
			t.autoRecord = !t.autoRecord
			changed = true
		}
	}
	if !changed {
		return
	}
	if t.autoArmed && !t.autoStarted && ctx != nil {
		t.autoStartTick = ctx.NowTick() + uint64(t.autoStartDelayMs)
	}
	if t.autoArmed && t.autoStarted && t.autoDurationMs > 0 {
		t.autoStopTick = t.autoRunStartTick + uint64(t.autoDurationMs)
	}
	t.invalidate(dirtyOverlay | dirtyStatus | dirtyRFControl)
}

func (t *Task) activateAutomationLine(ctx *kernel.Context) {
	switch t.autoSel {
	case 0:
		t.toggleAutomationArm(ctx)
		return
	case 1:
		t.openPrompt(promptAutoStartDelay, "Automation start delay (ms)", fmt.Sprintf("%d", t.autoStartDelayMs))
		return
	case 2:
		t.openPrompt(promptAutoDuration, "Automation duration (ms, 0=manual)", fmt.Sprintf("%d", t.autoDurationMs))
		return
	case 3:
		t.openPrompt(promptAutoStopSweeps, "Auto-stop after sweeps (0=off)", fmt.Sprintf("%d", t.autoStopSweeps))
		return
	case 4:
		t.openPrompt(promptAutoStopPackets, "Auto-stop after packets (0=off)", fmt.Sprintf("%d", t.autoStopPackets))
		return
	case 5:
		t.autoRecord = !t.autoRecord
		t.invalidate(dirtyOverlay | dirtyStatus | dirtyRFControl)
		return
	case 6:
		initial := t.autoSessionBase
		if strings.TrimSpace(initial) == "" {
			initial = "auto"
		}
		t.openPrompt(promptAutoName, "Automation session base name", initial)
		return
	}
}

func (t *Task) toggleAutomationArm(ctx *kernel.Context) {
	if t.autoArmed {
		t.disarmAutomation(ctx)
		return
	}
	t.armAutomation(ctx)
}

func (t *Task) armAutomation(ctx *kernel.Context) {
	if ctx == nil {
		return
	}
	now := ctx.NowTick()
	t.autoArmed = true
	t.autoStarted = false
	t.autoErr = ""
	t.autoStartTick = now + uint64(clampInt(t.autoStartDelayMs, 0, 1_000_000))
	t.autoStopTick = 0
	t.invalidate(dirtyStatus | dirtyRFControl)
}

func (t *Task) disarmAutomation(ctx *kernel.Context) {
	running := t.autoStarted
	t.autoArmed = false
	t.autoStarted = false
	t.autoStartTick = 0
	t.autoStopTick = 0
	t.autoRunStartTick = 0
	t.autoRunStartSweeps = 0
	t.autoRunStartPktSeq = 0
	t.autoErr = ""

	if running && ctx != nil {
		if t.recording {
			_ = t.stopRecording(ctx)
		}
		if t.scanActive {
			t.stopScan()
		}
	}
	t.invalidate(dirtyStatus | dirtyRFControl)
}

func (t *Task) onAutomationTick(ctx *kernel.Context, tick uint64) {
	if ctx == nil || t.replayActive || !t.autoArmed {
		return
	}
	if !t.autoStarted {
		if tick < t.autoStartTick {
			return
		}
		t.autoStarted = true
		t.autoRunStartTick = tick
		t.autoRunStartSweeps = t.sweepCount
		t.autoRunStartPktSeq = t.pktSeq
		t.autoErr = ""

		if !t.scanActive {
			t.startScan(tick)
		}
		if t.autoRecord && !t.recording {
			base := sanitizePresetName(strings.TrimSpace(t.autoSessionBase))
			if base == "" {
				base = "auto"
			}
			name := fmt.Sprintf("%s_%d", base, tick/1000)
			if err := t.startRecording(ctx, name); err != nil {
				t.autoErr = err.Error()
				t.autoArmed = false
				t.autoStarted = false
				t.invalidate(dirtyStatus | dirtyRFControl)
				return
			}
		}
		if t.autoDurationMs > 0 {
			t.autoStopTick = tick + uint64(t.autoDurationMs)
		} else {
			t.autoStopTick = 0
		}
		t.invalidate(dirtyStatus | dirtyRFControl)
		return
	}

	shouldStop := false
	if t.autoDurationMs > 0 && t.autoStopTick != 0 && tick >= t.autoStopTick {
		shouldStop = true
	}
	if !shouldStop && t.autoStopSweeps > 0 && t.sweepCount >= t.autoRunStartSweeps {
		if (t.sweepCount - t.autoRunStartSweeps) >= uint64(t.autoStopSweeps) {
			shouldStop = true
		}
	}
	if !shouldStop && t.autoStopPackets > 0 && t.pktSeq >= t.autoRunStartPktSeq {
		if int(t.pktSeq-t.autoRunStartPktSeq) >= t.autoStopPackets {
			shouldStop = true
		}
	}
	if !shouldStop && t.recordErr != "" {
		shouldStop = true
	}

	if !shouldStop {
		return
	}
	t.disarmAutomation(ctx)
}

func (t *Task) automationStatusLine(now uint64) string {
	if !t.autoArmed {
		return "AUTO: off"
	}
	if t.autoErr != "" {
		return "AUTO: ERR " + t.autoErr
	}
	if t.autoStarted {
		age := uint64(0)
		if now > t.autoRunStartTick {
			age = now - t.autoRunStartTick
		}
		stop := "manual"
		if t.autoStopTick != 0 && now < t.autoStopTick {
			stop = fmt.Sprintf("in %ds", int((t.autoStopTick-now)/1000))
		} else if t.autoStopTick != 0 && now >= t.autoStopTick {
			stop = "now"
		}
		return fmt.Sprintf("AUTO: RUN  age:%ds stop:%s", int(age/1000), stop)
	}
	if now < t.autoStartTick {
		return fmt.Sprintf("AUTO: ARMED  start in %ds", int((t.autoStartTick-now)/1000))
	}
	return "AUTO: ARMED  start now"
}
