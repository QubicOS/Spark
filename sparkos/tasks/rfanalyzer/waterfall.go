package rfanalyzer

func (t *Task) waterfallPlotRect(l layout) (plot rect, headerY int16, ok bool) {
	inner := l.waterfall.inset(2, 2)
	headerY = inner.y + t.fontHeight + 1
	plot = rect{
		x: inner.x,
		y: headerY + t.fontHeight + 1,
		w: inner.w,
		h: inner.h - 2*t.fontHeight - 3,
	}
	if plot.w <= 0 || plot.h <= 0 {
		return rect{}, 0, false
	}
	return plot, headerY, true
}

func (t *Task) ensureWaterfallAlloc() bool {
	if t.fb == nil {
		return false
	}
	l := t.computeLayout()
	plot, _, ok := t.waterfallPlotRect(l)
	if !ok {
		return false
	}
	plotW := int(plot.w)
	plotH := int(plot.h)
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
	lerpU8 := func(a, b uint8, t uint8) uint8 {
		return uint8((uint16(a)*(255-uint16(t)) + uint16(b)*uint16(t)) / 255)
	}
	lerpRGB := func(r0, g0, b0, r1, g1, b1, tt uint8) uint16 {
		return rgb565From888(
			lerpU8(r0, r1, tt),
			lerpU8(g0, g1, tt),
			lerpU8(b0, b1, tt),
		)
	}

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
	case wfPaletteCubic:
		// CubicSDR-like palette: deep blue background → cyan/green → yellow → red → white.
		for i := 0; i < 256; i++ {
			v := uint8(i)
			switch {
			case v < 64:
				// almost black → deep blue
				t.wfPalette565[i] = lerpRGB(0x00, 0x00, 0x06, 0x00, 0x00, 0x60, uint8(v*4))
			case v < 128:
				// deep blue → cyan
				t.wfPalette565[i] = lerpRGB(0x00, 0x00, 0x60, 0x00, 0xA0, 0xFF, uint8((v-64)*4))
			case v < 176:
				// cyan → green
				tw := uint8((uint16(v-128) * 255) / 48)
				t.wfPalette565[i] = lerpRGB(0x00, 0xA0, 0xFF, 0x00, 0xFF, 0x30, tw)
			case v < 208:
				// green → yellow
				tw := uint8((uint16(v-176) * 255) / 32)
				t.wfPalette565[i] = lerpRGB(0x00, 0xFF, 0x30, 0xFF, 0xFF, 0x00, tw)
			case v < 232:
				// yellow → red
				tw := uint8((uint16(v-208) * 255) / 24)
				t.wfPalette565[i] = lerpRGB(0xFF, 0xFF, 0x00, 0xFF, 0x20, 0x00, tw)
			default:
				// red → white
				tw := uint8((uint16(v-232) * 255) / 23)
				t.wfPalette565[i] = lerpRGB(0xFF, 0x20, 0x00, 0xFF, 0xFF, 0xFF, tw)
			}
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
