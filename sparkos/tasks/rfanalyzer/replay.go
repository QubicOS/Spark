package rfanalyzer

import (
	"encoding/binary"
	"errors"
	"fmt"

	"spark/sparkos/kernel"
)

func (t *Task) enterReplay(ctx *kernel.Context, input string) error {
	if ctx == nil {
		return errors.New("nil context")
	}
	if t.vfs == nil {
		return errors.New("vfs unavailable")
	}
	if t.recording {
		_ = t.stopRecording(ctx)
	}

	sess, err := t.loadSession(ctx, input)
	if err != nil {
		return err
	}

	t.replay = sess
	t.replayActive = true
	t.replayPlaying = false
	if t.replaySpeed <= 0 {
		t.replaySpeed = 1
	}
	t.replayHostLastTick = 0
	t.replayNowTick = sess.startTick
	t.replaySweepIdx = -1
	t.replayPktLimit = 0
	t.replayCfgIdx = -1
	t.replayErr = ""
	t.replayPktCacheOK = false
	t.replayPktCacheSeq = 0

	t.scanActive = false
	t.scanNextTick = 0
	t.capturePaused = false

	t.resetView()
	t.pktHead = 0
	t.pktCount = 0
	t.pktSeq = 0
	t.pktDropped = 0
	t.pktSecStart = 0
	t.pktSecCount = 0
	t.pktsPerSec = 0
	t.snifferSel = 0
	t.snifferTop = 0
	t.snifferSelSeq = 0

	t.applyReplayConfigAt(t.replayNowTick)
	t.updateReplayPosition(ctx, t.replayNowTick, true)
	t.invalidate(dirtyAll)
	return nil
}

func (t *Task) exitReplay(ctx *kernel.Context) {
	_ = ctx
	if !t.replayActive {
		return
	}
	t.replayActive = false
	t.replayPlaying = false
	t.replayHostLastTick = 0
	t.replayNowTick = 0
	t.replaySweepIdx = -1
	t.replayPktLimit = 0
	t.replayCfgIdx = -1
	t.replay = nil
	t.replayErr = ""
	t.replayPktCacheOK = false
	t.replayPktCacheSeq = 0
	t.invalidate(dirtyAll)
}

func (t *Task) onReplayTick(ctx *kernel.Context, hostTick uint64) {
	if !t.replayActive || t.replay == nil {
		return
	}

	if t.replayHostLastTick == 0 {
		t.replayHostLastTick = hostTick
	}
	delta := hostTick - t.replayHostLastTick
	t.replayHostLastTick = hostTick

	if t.replayPlaying {
		advance := delta * uint64(clampInt(t.replaySpeed, 1, 32))
		t.replayNowTick += advance
		if t.replayNowTick > t.replay.endTick {
			t.replayNowTick = t.replay.endTick
			t.replayPlaying = false
		}
	}

	t.applyReplayConfigAt(t.replayNowTick)
	t.updateReplayPosition(ctx, t.replayNowTick, false)
}

func (t *Task) updateReplayPacketCache(ctx *kernel.Context) {
	if !t.replayActive || t.replay == nil || ctx == nil || t.vfs == nil {
		return
	}
	meta, ok := t.filteredReplayPacketMetaByIndex(t.snifferSel)
	if !ok {
		t.replayPktCacheOK = false
		return
	}
	if t.replayPktCacheOK && t.replayPktCacheSeq == meta.seq {
		return
	}

	hdr, err := readAtExact(ctx, t.vfs, t.replay.path, meta.off, 5)
	if err != nil {
		t.replayErr = err.Error()
		t.replayPktCacheOK = false
		return
	}
	if sessionRecordType(hdr[0]) != recPacket {
		t.replayErr = "bad packet record"
		t.replayPktCacheOK = false
		return
	}
	recLen := binary.LittleEndian.Uint32(hdr[1:5])
	payload, err := readAtExact(ctx, t.vfs, t.replay.path, meta.off+5, int(recLen))
	if err != nil {
		t.replayErr = err.Error()
		t.replayPktCacheOK = false
		return
	}
	p, ok := decodeSessionPacket(payload)
	if !ok {
		t.replayErr = "bad packet payload"
		t.replayPktCacheOK = false
		return
	}
	t.replayPktCache = p
	t.replayPktCacheSeq = p.seq
	t.replayPktCacheOK = true
	t.invalidate(dirtyProtocol)
}

func (t *Task) updateReplayPosition(ctx *kernel.Context, sessionTick uint64, force bool) {
	if !t.replayActive || t.replay == nil {
		return
	}

	newSweepIdx := upperBoundSweep(t.replay.sweeps, sessionTick)
	if newSweepIdx < 0 {
		newSweepIdx = 0
	}
	if newSweepIdx >= len(t.replay.sweeps) {
		newSweepIdx = len(t.replay.sweeps) - 1
	}

	if force || newSweepIdx != t.replaySweepIdx {
		if force || newSweepIdx < t.replaySweepIdx || newSweepIdx-t.replaySweepIdx > 8 {
			t.replaySweepIdx = -1
			t.rebuildReplayWaterfall(ctx)
		}
		t.advanceReplaySweepTo(ctx, newSweepIdx)
		t.invalidate(dirtySpectrum | dirtyWaterfall | dirtyStatus)
	}

	newPktLimit := upperBoundPacket(t.replay.packets, sessionTick)
	if newPktLimit < 0 {
		newPktLimit = 0
	}
	if newPktLimit > len(t.replay.packets) {
		newPktLimit = len(t.replay.packets)
	}
	if force || newPktLimit != t.replayPktLimit {
		if newPktLimit > t.replayPktLimit {
			t.pktSecCount += newPktLimit - t.replayPktLimit
		}
		t.replayPktLimit = newPktLimit
		t.replayPktCacheOK = false
		t.reconcileSnifferSelection()
		t.invalidate(dirtySniffer | dirtyProtocol | dirtyStatus)
	}
}

func (t *Task) advanceReplaySweepTo(ctx *kernel.Context, target int) {
	if t.replay == nil || target < 0 || target >= len(t.replay.sweeps) {
		return
	}
	if t.replaySweepIdx < 0 {
		t.applyReplaySweep(ctx, target)
		t.replaySweepIdx = target
		return
	}
	if target <= t.replaySweepIdx {
		t.applyReplaySweep(ctx, target)
		t.replaySweepIdx = target
		return
	}

	for i := t.replaySweepIdx + 1; i <= target; i++ {
		t.applyReplaySweep(ctx, i)
		t.replaySweepIdx = i
		if !t.waterfallFrozen {
			t.pushWaterfallRow()
		}
	}
}

func (t *Task) rebuildReplayWaterfall(ctx *kernel.Context) {
	if !t.ensureWaterfallAlloc() || t.replay == nil || len(t.replay.sweeps) == 0 {
		return
	}
	for i := range t.wfBuf {
		t.wfBuf[i] = 0
	}
	t.wfHead = 0

	sweepIdx := t.replaySweepIdx
	if sweepIdx < 0 {
		sweepIdx = upperBoundSweep(t.replay.sweeps, t.replayNowTick)
		if sweepIdx < 0 {
			sweepIdx = 0
		}
	}

	start := sweepIdx - (t.wfH - 1)
	if start < 0 {
		start = 0
	}
	for i := start; i <= sweepIdx && i < len(t.replay.sweeps); i++ {
		t.applyReplaySweep(ctx, i)
		t.pushWaterfallRow()
	}
	t.invalidate(dirtyWaterfall)
}

func (t *Task) applyReplaySweep(ctx *kernel.Context, idx int) {
	if t.replay == nil || idx < 0 || idx >= len(t.replay.sweeps) {
		return
	}
	if t.vfs == nil || ctx == nil {
		return
	}

	off := t.replay.sweeps[idx].off
	hdr, err := readAtExact(ctx, t.vfs, t.replay.path, off, 5)
	if err != nil {
		t.replayErr = err.Error()
		return
	}
	if sessionRecordType(hdr[0]) != recSweep {
		t.replayErr = "bad sweep record"
		return
	}
	recLen := binary.LittleEndian.Uint32(hdr[1:5])
	payload, err := readAtExact(ctx, t.vfs, t.replay.path, off+5, int(recLen))
	if err != nil {
		t.replayErr = err.Error()
		return
	}
	if len(payload) < 8+numChannels {
		t.replayErr = "short sweep payload"
		return
	}

	for ch := 0; ch < numChannels; ch++ {
		v := payload[8+ch]
		t.energyCur[ch] = v
		t.energyAvg[ch] = v
		if t.energyPeak[ch] > 0 {
			t.energyPeak[ch]--
		}
		if v > t.energyPeak[ch] {
			t.energyPeak[ch] = v
		}
	}
	t.sweepCount = uint64(idx + 1)
}

func (t *Task) applyReplayConfigAt(sessionTick uint64) {
	if !t.replayActive || t.replay == nil || len(t.replay.configs) == 0 {
		return
	}
	i := upperBoundConfig(t.replay.configs, sessionTick)
	if i < 0 {
		i = 0
	}
	if i >= len(t.replay.configs) {
		i = len(t.replay.configs) - 1
	}
	if i == t.replayCfgIdx {
		return
	}
	t.replayCfgIdx = i
	ev := t.replay.configs[i]

	t.applyConfig(ev.cfg)
	t.selectedChannel = clampInt(ev.selectedChannel, 0, maxChannel)
	t.presetDirty = false
}

func upperBoundSweep(sweeps []sweepIndex, tick uint64) int {
	lo := 0
	hi := len(sweeps)
	for lo < hi {
		mid := lo + (hi-lo)/2
		if sweeps[mid].tick <= tick {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo - 1
}

func upperBoundPacket(pkts []packetMeta, tick uint64) int {
	lo := 0
	hi := len(pkts)
	for lo < hi {
		mid := lo + (hi-lo)/2
		if pkts[mid].tick <= tick {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo
}

func upperBoundConfig(cfgs []sessionConfigEvent, tick uint64) int {
	lo := 0
	hi := len(cfgs)
	for lo < hi {
		mid := lo + (hi-lo)/2
		if cfgs[mid].tick <= tick {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo - 1
}

func (t *Task) replayTimeText() string {
	if t.replay == nil {
		return ""
	}
	if t.replayNowTick < t.replay.startTick {
		return "t:-"
	}
	return fmt.Sprintf("t:%ds", int((t.replayNowTick-t.replay.startTick)/1000))
}
