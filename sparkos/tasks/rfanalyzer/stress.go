package rfanalyzer

import (
	"encoding/binary"

	"spark/sparkos/kernel"
)

func (t *Task) startStress(now uint64) {
	t.stressRunning = true
	t.stressStartTick = now
	t.stressNextTick = now
	t.stressSent = 0
	t.stressRecv = 0
	t.stressLost = 0
	t.stressLatAvgMs = 0
	t.stressLatMaxMs = 0
	t.invalidate(dirtyAnalysis | dirtyStatus)
}

func (t *Task) stopStress() {
	t.stressRunning = false
	t.stressNextTick = 0
	t.invalidate(dirtyAnalysis | dirtyStatus)
}

func (t *Task) onStressTick(_ *kernel.Context, now uint64) {
	if t.replayActive || !t.stressRunning {
		return
	}
	if t.stressStartTick == 0 {
		t.startStress(now)
		return
	}
	if t.stressDurationMs > 0 && now > t.stressStartTick {
		if (now - t.stressStartTick) >= uint64(t.stressDurationMs) {
			t.stopStress()
			return
		}
	}

	pps := clampInt(t.stressPPS, 1, 1000)
	interval := uint64(1000 / pps)
	if interval == 0 {
		interval = 1
	}

	const maxOpsPerTick = 6
	for ops := 0; ops < maxOpsPerTick && t.stressNextTick != 0 && now >= t.stressNextTick; ops++ {
		t.stressNextTick += interval
		t.stressSent++

		ch := clampInt(t.selectedChannel, 0, maxChannel)
		energy := t.energyAvg[ch]

		// xorshift32
		x := t.rng
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		t.rng = x

		dropPct := 5
		switch {
		case energy >= 220:
			dropPct = 80
		case energy >= 200:
			dropPct = 60
		case energy >= 180:
			dropPct = 40
		case energy >= 160:
			dropPct = 25
		case energy >= 120:
			dropPct = 12
		}
		if int(x&0xFF) < dropPct*256/100 {
			t.stressLost++
			continue
		}

		lat := uint32(1 + int(energy)/64 + int(x&0x07))
		t.stressRecv++
		if t.stressRecv == 1 {
			t.stressLatAvgMs = lat
		} else {
			avg := t.stressLatAvgMs
			avg += (lat - avg) / t.stressRecv
			t.stressLatAvgMs = avg
		}
		if lat > t.stressLatMaxMs {
			t.stressLatMaxMs = lat
		}

		p := packet{
			tick:    now,
			channel: uint8(ch),
			rate:    t.dataRate,
			addrLen: 5,
			addr:    [5]byte{0xD1, 0xA6, 0xE5, 0x5A, 0x01},
			length:  16,
			crcLen:  2,
			crcOK:   true,
		}
		var b [4]byte
		binary.LittleEndian.PutUint32(b[:], t.stressSent)
		copy(p.payload[0:4], b[:])
		binary.LittleEndian.PutUint32(b[:], lat)
		copy(p.payload[4:8], b[:])
		copy(p.payload[8:16], []byte("STRESSOK"))
		t.appendPacket(p)
	}
}
