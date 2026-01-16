package quarkgl

// Color is an RGBA color in 8-bit channels.
type Color struct {
	R, G, B, A uint8
}

func RGB(r, g, b uint8) Color     { return Color{R: r, G: g, B: b, A: 0xFF} }
func RGBA(r, g, b, a uint8) Color { return Color{R: r, G: g, B: b, A: a} }

func (c Color) MulScalar(s Scalar) Color {
	// Scalar is float32 (default) or fixed-point. Convert through 0..1 as needed.
	var t uint32
	switch any(s).(type) {
	case float32:
		fs := float32(s)
		if fs < 0 {
			fs = 0
		}
		if fs > 1 {
			fs = 1
		}
		t = uint32(fs * 255)
	default:
		// fixed Q16.16
		v := int32(s)
		if v < 0 {
			v = 0
		}
		if v > (1 << 16) {
			v = 1 << 16
		}
		t = uint32((int64(v) * 255) >> 16)
	}

	mul := func(ch uint8) uint8 {
		return uint8((uint32(ch) * t) / 255)
	}
	return Color{R: mul(c.R), G: mul(c.G), B: mul(c.B), A: c.A}
}

func (c Color) WithAlpha(a uint8) Color { c.A = a; return c }
