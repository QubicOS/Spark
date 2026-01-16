package rfanalyzer

import (
	"fmt"

	"spark/sparkos/kernel"
)

const renderIntervalTicks = 33

func (t *Task) onTick(ctx *kernel.Context, tick uint64) {
	if t.replayActive {
		t.onReplayTick(ctx, tick)
		return
	}
	if !t.scanActive {
		return
	}

	now := tick
	if t.scanNextTick == 0 || t.scanChan < t.channelRangeLo || t.scanChan > t.channelRangeHi {
		t.scanChan = t.channelRangeLo
		t.scanNextTick = now
	}

	dwell := uint64(t.dwellTimeMs)
	if dwell == 0 {
		dwell = 1
	}
	step := clampInt(t.scanSpeedScalar, 1, 10)

	const maxOpsPerTick = 8
	for ops := 0; ops < maxOpsPerTick && now >= t.scanNextTick; ops++ {
		ch := t.scanChan
		if ch < t.channelRangeLo || ch > t.channelRangeHi {
			ch = t.channelRangeLo
		}
		if ch < 0 {
			ch = 0
		}
		if ch > maxChannel {
			ch = maxChannel
		}

		v := t.sampleEnergy(ch, now)
		t.updateChannel(ch, v)
		t.maybeCapturePacket(ch, v, now)
		t.invalidate(dirtySpectrum)

		t.scanChan += step
		if t.scanChan > t.channelRangeHi {
			t.scanChan = t.channelRangeLo
			t.onSweepComplete(now)
		}

		t.scanNextTick += dwell
	}
}

func (t *Task) startScan(now uint64) {
	t.scanActive = true
	t.scanChan = t.channelRangeLo
	t.scanNextTick = now
	t.invalidate(dirtyStatus | dirtySpectrum | dirtyWaterfall)
}

func (t *Task) stopScan() {
	t.scanActive = false
	t.scanNextTick = 0
	t.invalidate(dirtyStatus)
}

func (t *Task) updateChannel(ch int, v uint8) {
	if ch < 0 || ch >= numChannels {
		return
	}
	t.energyCur[ch] = v

	avg := int(t.energyAvg[ch])
	cur := int(v)
	avg += (cur - avg) / 4
	if avg < 0 {
		avg = 0
	}
	if avg > 255 {
		avg = 255
	}
	t.energyAvg[ch] = uint8(avg)

	if v > t.energyPeak[ch] {
		t.energyPeak[ch] = v
	}
}

func (t *Task) onSweepComplete(now uint64) {
	t.sweepCount++
	t.lastSweepTick = now

	for i := 0; i < numChannels; i++ {
		if t.energyPeak[i] > 0 {
			t.energyPeak[i]--
		}
		if t.energyAvg[i] > 0 && (t.scanSpeedScalar > 1) {
			t.energyAvg[i]--
		}
	}

	t.recordSweep(now)
	t.analyticsOnSweep(now)

	if t.waterfallFrozen {
		return
	}
	if !t.ensureWaterfallAlloc() {
		return
	}
	t.pushWaterfallRow()
	t.invalidate(dirtyWaterfall | dirtyStatus)
}

func (t *Task) sampleEnergy(ch int, tick uint64) uint8 {
	if t.rng == 0 {
		t.rng = 0xA341316C
	}
	// xorshift32
	x := t.rng
	x ^= x << 13
	x ^= x >> 17
	x ^= x << 5
	t.rng = x

	noise := int(x&0x0F) + 4
	v := noise

	// Wi‑Fi-like wide signals at 2412/2437/2462 MHz (channels 12/37/62).
	v += bump(ch, 12, 12, toggleAmp(tick, 650, 220, 70))
	v += bump(ch, 37, 12, toggleAmp(tick, 480, 200, 60))
	v += bump(ch, 62, 12, toggleAmp(tick, 720, 210, 80))

	// Bluetooth-like narrowband hopping.
	btCenter := 2 + int((tick/11)%80)
	v += bump(ch, btCenter, 3, 170)

	// A couple of nRF24-like narrow carriers.
	v += bump(ch, 76, 2, toggleAmp(tick, 900, 240, 0))
	v += bump(ch, 91, 2, toggleAmp(tick, 1100, 180, 0))

	if v < 0 {
		v = 0
	}
	if v > 255 {
		v = 255
	}
	return uint8(v)
}

func bump(ch, center, width, amp int) int {
	if width <= 0 || amp <= 0 {
		return 0
	}
	d := ch - center
	if d < 0 {
		d = -d
	}
	if d > width {
		return 0
	}
	return amp * (width - d) / width
}

func toggleAmp(tick uint64, periodTicks uint64, onAmp, offAmp int) int {
	if periodTicks == 0 {
		return onAmp
	}
	if (tick % periodTicks) < periodTicks*2/3 {
		return onAmp
	}
	return offAmp
}

func (t *Task) statusLine() string {
	mode := "IDLE"
	if t.replayActive {
		mode = "REPLAY"
	} else if t.scanActive {
		mode = "SCAN"
	}
	wf := "RUN"
	if t.waterfallFrozen {
		wf = "FROZEN"
	}
	cap := "LIVE"
	if t.replayActive {
		if t.replayPlaying {
			cap = "PLAY"
		} else {
			cap = "PAUSE"
		}
	} else if t.capturePaused {
		cap = "PAUSED"
	}

	rec := "OFF"
	if t.recording {
		rec = "ON"
	}
	recInfo := "REC:" + rec
	if t.recordErr != "" {
		recInfo = "REC:ERR"
	}
	if t.recording && t.recordName != "" {
		recInfo = fmt.Sprintf("REC:%s %s %dKB", rec, t.recordName, t.recordBytes/1024)
	}

	extra := ""
	if t.replayActive {
		extra = fmt.Sprintf("  %s x%d", t.replayTimeText(), clampInt(t.replaySpeed, 1, 32))
	}
	auto := ""
	if t.autoArmed {
		if t.autoErr != "" {
			auto = " AUTO:ERR"
		} else if t.autoStarted {
			auto = " AUTO:RUN"
		} else {
			auto = " AUTO:ARM"
		}
	}
	return fmt.Sprintf("MODE:%s WF:%s CAP:%s %s%s%s  SEL:%03d  RATE:%s CRC:%s  PKT/s:%d DROP:%d", mode, wf, cap, recInfo, auto, extra, t.selectedChannel, t.dataRate, t.crcMode, t.pktsPerSec, t.pktDropped)
}
