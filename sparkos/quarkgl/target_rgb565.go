package quarkgl

// RGB565Target renders into an RGB565 framebuffer buffer.
//
// This type is intentionally simple and requires no Spark services; callers provide
// the backing buffer and layout (stride).
type RGB565Target struct {
	Buf    []byte
	Stride int // bytes per row
	W      int
	H      int
}

func (t *RGB565Target) Size() (w, h int) { return t.W, t.H }

func (t *RGB565Target) Clear(c Color) {
	if t == nil || t.Buf == nil || t.Stride <= 0 || t.W <= 0 || t.H <= 0 {
		return
	}
	p := rgb565From888(c.R, c.G, c.B)
	lo := byte(p)
	hi := byte(p >> 8)
	for y := 0; y < t.H; y++ {
		row := y * t.Stride
		for x := 0; x < t.W; x++ {
			off := row + x*2
			if off < 0 || off+1 >= len(t.Buf) {
				continue
			}
			t.Buf[off] = lo
			t.Buf[off+1] = hi
		}
	}
}

func (t *RGB565Target) SetPixel(x, y int, c Color) {
	if t == nil || t.Buf == nil || t.Stride <= 0 || t.W <= 0 || t.H <= 0 {
		return
	}
	if x < 0 || y < 0 || x >= t.W || y >= t.H {
		return
	}
	off := y*t.Stride + x*2
	if off < 0 || off+1 >= len(t.Buf) {
		return
	}
	p := rgb565From888(c.R, c.G, c.B)
	t.Buf[off] = byte(p)
	t.Buf[off+1] = byte(p >> 8)
}

func rgb565From888(r, g, b uint8) uint16 {
	return uint16((uint16(r>>3)&0x1F)<<11 | (uint16(g>>2)&0x3F)<<5 | (uint16(b>>3) & 0x1F))
}
