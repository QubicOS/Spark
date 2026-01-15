package rfanalyzer

func (t *Task) ensureWaterfallAlloc() bool {
	if t.fb == nil {
		return false
	}
	l := t.computeLayout()
	inner := l.waterfall.inset(2, 2)
	plotW := int(inner.w)
	plotH := int(inner.h - t.fontHeight - 2)
	if plotW <= 0 || plotH <= 0 {
		return false
	}
	if t.wfW != plotW || t.wfH != plotH || t.wfBuf == nil {
		t.wfW = plotW
		t.wfH = plotH
		t.wfHead = 0
		t.wfBuf = make([]uint8, plotW*plotH)
	}
	t.rebuildWaterfallPalette()
	return true
}

func (t *Task) pushWaterfallRow() {
	if t.wfW <= 0 || t.wfH <= 0 || t.wfBuf == nil {
		return
	}
	row := t.wfHead
	base := row * t.wfW
	if base < 0 || base+t.wfW > len(t.wfBuf) {
		return
	}

	for x := 0; x < t.wfW; x++ {
		lo := x * numChannels / t.wfW
		hi := (x+1)*numChannels/t.wfW - 1
		if lo < 0 {
			lo = 0
		}
		if hi < lo {
			hi = lo
		}
		if hi >= numChannels {
			hi = numChannels - 1
		}

		mx := uint8(0)
		for ch := lo; ch <= hi; ch++ {
			if t.energyAvg[ch] > mx {
				mx = t.energyAvg[ch]
			}
		}
		t.wfBuf[base+x] = mx
	}

	t.wfHead++
	if t.wfHead >= t.wfH {
		t.wfHead = 0
	}
}

func (t *Task) rebuildWaterfallPalette() {
	switch t.wfPalette {
	case wfPaletteFire:
		for i := 0; i < 256; i++ {
			v := uint8(i)
			r := v
			g := uint8(0)
			b := uint8(0)
			if v > 64 {
				g = clampByte(int(v-64) * 2)
			}
			if v > 160 {
				b = clampByte(int(v-160) * 2)
			}
			t.wfPalette565[i] = rgb565From888(r, g, b)
		}
	case wfPaletteGray:
		for i := 0; i < 256; i++ {
			v := uint8(i)
			t.wfPalette565[i] = rgb565From888(v, v, v)
		}
	default: // CYAN
		for i := 0; i < 256; i++ {
			v := uint8(i)
			r := uint8(0)
			g := uint8(int(v) * 7 / 8)
			b := v
			if v > 200 {
				r = clampByte(int(v-200) * 2)
			}
			t.wfPalette565[i] = rgb565From888(r, g, b)
		}
	}
}

func clampByte(v int) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}
