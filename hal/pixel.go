package hal

func rgb565(r, g, b uint8) uint16 {
	rr := uint16(r>>3) & 0x1F
	gg := uint16(g>>2) & 0x3F
	bb := uint16(b>>3) & 0x1F
	return (rr << 11) | (gg << 5) | bb
}

func rgb888From565(p uint16) (r, g, b uint8) {
	rr := (p >> 11) & 0x1F
	gg := (p >> 5) & 0x3F
	bb := p & 0x1F

	r = uint8((rr * 255) / 31)
	g = uint8((gg * 255) / 63)
	b = uint8((bb * 255) / 31)
	return r, g, b
}
