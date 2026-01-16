package rfanalyzer

import "fmt"

const (
	anaOccThreshold         = 160
	anaRetryWindowTicks     = 60
	anaBestIntervalTicks    = 10_000
	anaPeriodicMinIntervals = 3
	maxDevices              = 64
)

type analysisView uint8

const (
	analysisChannels analysisView = iota
	analysisDevices
	analysisTiming
	analysisCollisions
)

func (v analysisView) String() string {
	switch v {
	case analysisChannels:
		return "CHAN"
	case analysisDevices:
		return "DEV"
	case analysisTiming:
		return "TIME"
	case analysisCollisions:
		return "COL"
	default:
		return "?"
	}
}

type bestChanEntry struct {
	tick  uint64
	ch    uint8
	score uint8
}

type deviceStat struct {
	used bool

	addrLen uint8
	addr    [5]byte

	firstTick uint64
	lastTick  uint64

	lastChannel uint8
	hopCount    uint32

	pktCount uint32
	crcOK    uint32
	crcBad   uint32

	retries         uint32
	lastPayloadHash uint32
	lastPayloadTick uint64

	intCount   uint32
	intAvg     uint32
	intJitter  uint32
	intMin     uint32
	intMax     uint32
	burstCur   uint16
	burstMax   uint16
	burstCount uint32
}

func (d *deviceStat) addrSuffix3() string {
	if d == nil || d.addrLen == 0 {
		return "------"
	}
	return addrSuffix3(d.addrLen, d.addr)
}

func (t *Task) resetAnalytics() {
	t.anaSweepCount = 0
	for i := 0; i < numChannels; i++ {
		t.anaOccCount[i] = 0
		t.anaEnergySum[i] = 0
		t.anaChanPkt[i] = 0
		t.anaChanBad[i] = 0
		t.anaChanRetry[i] = 0
		t.anaHigh[i] = false
		t.anaLastRise[i] = 0
		t.anaRiseCount[i] = 0
		t.anaRiseAvgMs[i] = 0
		t.anaRiseMinMs[i] = 0
		t.anaRiseMaxMs[i] = 0
	}

	for i := range t.bestHist {
		t.bestHist[i] = bestChanEntry{}
	}
	t.bestHead = 0
	t.bestCount = 0
	t.bestNextTick = 0

	for i := range t.devices {
		t.devices[i] = deviceStat{}
	}
	t.deviceCount = 0

	t.analysisSel = 0
	t.analysisTop = 0
	t.invalidate(dirtyAnalysis)
}

func (t *Task) analyticsOnSweep(tick uint64) {
	t.anaSweepCount++
	for ch := 0; ch < numChannels; ch++ {
		v := t.energyAvg[ch]
		t.anaEnergySum[ch] += uint32(v)
		if v >= anaOccThreshold {
			t.anaOccCount[ch]++
		}

		high := v >= anaOccThreshold
		if high && !t.anaHigh[ch] {
			if last := t.anaLastRise[ch]; last != 0 && tick > last {
				dt := tick - last
				ms := uint32(dt)
				if ms == 0 {
					ms = 1
				}
				cnt := t.anaRiseCount[ch]
				if cnt < 255 {
					cnt++
				}
				t.anaRiseCount[ch] = cnt
				if cnt == 1 {
					t.anaRiseAvgMs[ch] = ms
					t.anaRiseMinMs[ch] = ms
					t.anaRiseMaxMs[ch] = ms
				} else {
					avg := t.anaRiseAvgMs[ch]
					avg += (ms - avg) / uint32(cnt)
					t.anaRiseAvgMs[ch] = avg
					if ms < t.anaRiseMinMs[ch] {
						t.anaRiseMinMs[ch] = ms
					}
					if ms > t.anaRiseMaxMs[ch] {
						t.anaRiseMaxMs[ch] = ms
					}
				}
			}
			t.anaLastRise[ch] = tick
		}
		t.anaHigh[ch] = high
	}

	if t.bestNextTick == 0 || tick >= t.bestNextTick {
		ch, score := t.bestChannelNow()
		if score > 0 {
			t.bestHist[t.bestHead] = bestChanEntry{tick: tick, ch: uint8(ch), score: uint8(score)}
			t.bestHead++
			if t.bestHead >= len(t.bestHist) {
				t.bestHead = 0
			}
			if t.bestCount < len(t.bestHist) {
				t.bestCount++
			}
		}
		t.bestNextTick = tick + anaBestIntervalTicks
	}

	t.invalidate(dirtyAnalysis)
}

func (t *Task) analyticsOnPacket(p *packet) {
	if p == nil {
		return
	}
	hash := fnv1a32(p.payload[:p.length])
	t.analyticsOnPacketMeta(packetMeta{
		tick:        p.tick,
		seq:         p.seq,
		channel:     p.channel,
		rate:        p.rate,
		addrLen:     p.addrLen,
		addr:        p.addr,
		length:      p.length,
		payloadHash: hash,
		crcLen:      p.crcLen,
		crcOK:       p.crcOK,
	})
}

func (t *Task) analyticsOnPacketMeta(m packetMeta) {
	ch := int(m.channel)
	if ch < 0 || ch >= numChannels {
		return
	}
	t.anaChanPkt[ch]++
	if m.crcLen > 0 && !m.crcOK {
		t.anaChanBad[ch]++
	}

	d := t.findOrAllocDevice(m.addrLen, m.addr, m.tick)
	if d == nil {
		t.invalidate(dirtyAnalysis)
		return
	}

	if d.pktCount == 0 {
		d.firstTick = m.tick
		d.intMin = 0
		d.intMax = 0
	}
	if d.lastTick != 0 && m.tick > d.lastTick {
		dt := uint32(m.tick - d.lastTick)
		if dt != 0 {
			d.intCount++
			if d.intCount == 1 {
				d.intAvg = dt
				d.intMin = dt
				d.intMax = dt
			} else {
				avg := d.intAvg
				avg += (dt - avg) / d.intCount
				d.intAvg = avg
				if dt < d.intMin {
					d.intMin = dt
				}
				if dt > d.intMax {
					d.intMax = dt
				}
				j := absDiffU32(dt, avg)
				d.intJitter += (j - d.intJitter) / d.intCount
			}

			// Burst: consecutive short intervals.
			if dt <= 12 {
				d.burstCur++
				if d.burstCur > d.burstMax {
					d.burstMax = d.burstCur
				}
			} else if d.burstCur > 0 {
				d.burstCount++
				d.burstCur = 0
			}
		}
	}

	retry := false
	if d.lastPayloadHash != 0 && d.lastPayloadHash == m.payloadHash && m.tick > d.lastPayloadTick && (m.tick-d.lastPayloadTick) <= anaRetryWindowTicks {
		retry = true
	}
	d.lastPayloadHash = m.payloadHash
	d.lastPayloadTick = m.tick
	if retry {
		d.retries++
		t.anaChanRetry[ch]++
	}

	if d.pktCount > 0 && d.lastChannel != m.channel {
		d.hopCount++
	}
	d.lastChannel = m.channel

	d.pktCount++
	if m.crcLen > 0 && !m.crcOK {
		d.crcBad++
	} else if m.crcLen > 0 {
		d.crcOK++
	}

	d.lastTick = m.tick
	t.invalidate(dirtyAnalysis)
}

func absDiffU32(a, b uint32) uint32 {
	if a >= b {
		return a - b
	}
	return b - a
}

type chanScore struct {
	ch    int
	score int
}

func (t *Task) channelScore(ch int) int {
	if ch < 0 || ch >= numChannels {
		return 0
	}
	if t.anaSweepCount == 0 {
		return 0
	}
	occPct := int(t.anaOccCount[ch] * 100 / uint32(t.anaSweepCount))
	avgEnergy := int(t.anaEnergySum[ch] / uint32(t.anaSweepCount))

	pkt := t.anaChanPkt[ch]
	badPct := 0
	retryPct := 0
	if pkt > 0 {
		badPct = int(t.anaChanBad[ch] * 100 / pkt)
		retryPct = int(t.anaChanRetry[ch] * 100 / pkt)
	}

	score := 100
	score -= occPct * 50 / 100
	score -= avgEnergy * 30 / 255
	score -= badPct * 15 / 100
	score -= retryPct * 5 / 100
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score
}

func (t *Task) bestChannelNow() (int, int) {
	bestCh := 0
	bestScore := -1
	for ch := 0; ch < numChannels; ch++ {
		s := t.channelScore(ch)
		if s > bestScore {
			bestScore = s
			bestCh = ch
		}
	}
	if bestScore < 0 {
		bestScore = 0
	}
	return bestCh, bestScore
}

func (t *Task) topChannels(n int) []chanScore {
	if n < 1 {
		return nil
	}
	if n > numChannels {
		n = numChannels
	}
	top := make([]chanScore, 0, n)
	for ch := 0; ch < numChannels; ch++ {
		s := t.channelScore(ch)
		if len(top) < n {
			top = append(top, chanScore{ch: ch, score: s})
			for j := len(top) - 1; j > 0 && top[j].score > top[j-1].score; j-- {
				top[j], top[j-1] = top[j-1], top[j]
			}
			continue
		}
		if s <= top[len(top)-1].score {
			continue
		}
		top[len(top)-1] = chanScore{ch: ch, score: s}
		for j := len(top) - 1; j > 0 && top[j].score > top[j-1].score; j-- {
			top[j], top[j-1] = top[j-1], top[j]
		}
	}
	return top
}

func (t *Task) periodicText(ch int) string {
	if ch < 0 || ch >= numChannels {
		return ""
	}
	cnt := t.anaRiseCount[ch]
	if int(cnt) < anaPeriodicMinIntervals {
		return ""
	}
	avg := t.anaRiseAvgMs[ch]
	min := t.anaRiseMinMs[ch]
	max := t.anaRiseMaxMs[ch]
	if avg == 0 {
		return ""
	}
	j := max - min
	if j > avg/4 {
		return ""
	}
	return fmt.Sprintf("P~%dms", avg)
}

func (t *Task) findOrAllocDevice(addrLen uint8, addr [5]byte, tick uint64) *deviceStat {
	if addrLen == 0 {
		return nil
	}
	if addrLen > 5 {
		addrLen = 5
	}
	if d := t.findDevice(addrLen, addr); d != nil {
		return d
	}

	// Allocate slot: first free, else evict oldest.
	evict := -1
	var oldest uint64
	for i := range t.devices {
		if !t.devices[i].used {
			evict = i
			break
		}
		if evict == -1 || t.devices[i].lastTick < oldest {
			oldest = t.devices[i].lastTick
			evict = i
		}
	}
	if evict < 0 {
		return nil
	}
	d := &t.devices[evict]
	*d = deviceStat{}
	d.used = true
	d.addrLen = addrLen
	d.addr = addr
	d.firstTick = tick
	d.lastTick = tick
	t.deviceCount = t.countDevicesUsed()
	return d
}

func (t *Task) findDevice(addrLen uint8, addr [5]byte) *deviceStat {
	if addrLen == 0 {
		return nil
	}
	if addrLen > 5 {
		addrLen = 5
	}
	for i := range t.devices {
		d := &t.devices[i]
		if !d.used || d.addrLen != addrLen {
			continue
		}
		match := true
		for j := 0; j < int(addrLen); j++ {
			if d.addr[j] != addr[j] {
				match = false
				break
			}
		}
		if match {
			return d
		}
	}
	return nil
}

func (t *Task) countDevicesUsed() int {
	n := 0
	for i := range t.devices {
		if t.devices[i].used {
			n++
		}
	}
	return n
}

func (t *Task) topDeviceIndices(n int) []int {
	if n < 1 {
		return nil
	}
	out := make([]int, 0, n)
	for i := range t.devices {
		d := &t.devices[i]
		if !d.used || d.pktCount == 0 {
			continue
		}
		if len(out) < n {
			out = append(out, i)
			for j := len(out) - 1; j > 0 && t.devices[out[j]].pktCount > t.devices[out[j-1]].pktCount; j-- {
				out[j], out[j-1] = out[j-1], out[j]
			}
			continue
		}
		if d.pktCount <= t.devices[out[len(out)-1]].pktCount {
			continue
		}
		out[len(out)-1] = i
		for j := len(out) - 1; j > 0 && t.devices[out[j]].pktCount > t.devices[out[j-1]].pktCount; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func (t *Task) topConflictChannels(n int) []string {
	if n < 1 {
		return nil
	}
	type row struct {
		ch       int
		badPct   int
		retryPct int
		pkt      uint32
	}
	top := make([]row, 0, n)
	for ch := 0; ch < numChannels; ch++ {
		pkt := t.anaChanPkt[ch]
		if pkt < 8 {
			continue
		}
		badPct := int(t.anaChanBad[ch] * 100 / pkt)
		retryPct := int(t.anaChanRetry[ch] * 100 / pkt)
		score := badPct*2 + retryPct
		if score == 0 {
			continue
		}
		r := row{ch: ch, badPct: badPct, retryPct: retryPct, pkt: pkt}
		if len(top) < n {
			top = append(top, r)
		} else {
			// replace worst
			worst := 0
			worstScore := top[0].badPct*2 + top[0].retryPct
			for i := 1; i < len(top); i++ {
				s := top[i].badPct*2 + top[i].retryPct
				if s < worstScore {
					worstScore = s
					worst = i
				}
			}
			if score <= worstScore {
				continue
			}
			top[worst] = r
		}
	}
	// sort descending by score
	for i := 0; i < len(top); i++ {
		for j := i + 1; j < len(top); j++ {
			si := top[i].badPct*2 + top[i].retryPct
			sj := top[j].badPct*2 + top[j].retryPct
			if sj > si {
				top[i], top[j] = top[j], top[i]
			}
		}
	}
	lines := make([]string, 0, len(top))
	for _, r := range top {
		line := fmt.Sprintf("ch:%03d  pkt:%4d  bad:%2d%%  rty:%2d%%", r.ch, r.pkt, r.badPct, r.retryPct)
		lines = append(lines, line)
	}
	return lines
}
