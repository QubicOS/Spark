package rfanalyzer

func (t *Task) updateTickStats(now uint64) {
	if now == 0 {
		return
	}
	if t.tickStatsLast == 0 {
		t.tickStatsLast = now
		return
	}
	if now <= t.tickStatsLast {
		t.tickStatsLast = now
		return
	}
	dt := uint32(now - t.tickStatsLast)
	t.tickStatsLast = now
	if dt == 0 {
		return
	}

	t.tickStatsCount++
	if t.tickStatsCount == 1 {
		t.tickStatsAvgMs = dt
		t.tickStatsMinMs = dt
		t.tickStatsMaxMs = dt
		return
	}

	avg := t.tickStatsAvgMs
	avg += (dt - avg) / t.tickStatsCount
	t.tickStatsAvgMs = avg
	if t.tickStatsMinMs == 0 || dt < t.tickStatsMinMs {
		t.tickStatsMinMs = dt
	}
	if dt > t.tickStatsMaxMs {
		t.tickStatsMaxMs = dt
	}
}

func (t *Task) runDiagnostics(now uint64) {
	t.diagLastRunTick = now

	// RF: basic sanity (scan + sweep values changing).
	sum := 0
	for i := range t.energyAvg {
		sum += int(t.energyAvg[i])
	}
	t.diagRFOK = sum > 0 || t.scanActive

	// SPI/RF IO stubs: until a real nRF24 driver is wired in.
	t.diagSPIOK = true

	// Timing: tick jitter.
	jitter := int32(t.tickStatsMaxMs) - int32(t.tickStatsMinMs)
	if t.tickStatsCount == 0 {
		t.diagTimingOK = false
	} else {
		t.diagTimingOK = jitter <= 2
	}

	// Stability: coarse score.
	score := 100
	if t.recordErr != "" {
		score -= 40
	}
	if t.pktDropped > 0 {
		score -= 20
	}
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	t.diagStabilityScore = score

	t.invalidate(dirtyAnalysis | dirtyStatus)
}
