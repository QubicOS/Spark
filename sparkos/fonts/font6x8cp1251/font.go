package font6x8cp1251

import (
	"image/color"

	"tinygo.org/x/drivers"
	"tinygo.org/x/tinyfont"
)

// Font is the system monospace bitmap font (6x8) with CP1251 coverage.
//
// It implements tinyfont.Fonter so it can be used by tinyterm and in-app renderers.
// Concurrent access is not safe due to internal glyph reuse.
var Font tinyfont.Fonter = &font6x8{}

type font6x8 struct {
	g glyph
}

type glyph struct {
	r rune
}

func (g *glyph) Draw(display drivers.Displayer, x, y int16, c color.RGBA) {
	idx := glyphIndex(g.r)
	if idx < 0 {
		return
	}

	base := idx * 8
	for row := 0; row < 8; row++ {
		b := glyphData[base+row]
		// Bits are stored as 0b00xxxxxx (bit5 = leftmost pixel).
		for col := 0; col < 6; col++ {
			if b&(0x20>>col) == 0 {
				continue
			}
			display.SetPixel(x+int16(col), y-int16(7-row), c)
		}
	}
}

func (g *glyph) Info() tinyfont.GlyphInfo {
	return tinyfont.GlyphInfo{
		Rune:     g.r,
		Width:    6,
		Height:   8,
		XAdvance: 6,
		XOffset:  0,
		YOffset:  -7,
	}
}

func (f *font6x8) GetYAdvance() uint8 { return 8 }

func (f *font6x8) GetGlyph(r rune) tinyfont.Glypher {
	f.g.r = r
	return &f.g
}

func glyphIndex(r rune) int {
	b, ok := runeToCP1251(r)
	if !ok {
		b = '?'
	}
	if b < 0x20 {
		b = 0x20
	}
	idx := int(b) - 0x20
	if idx < 0 || idx >= 224 {
		return int('?' - 0x20)
	}
	return idx
}

func runeToCP1251(r rune) (byte, bool) {
	if r >= 0x20 && r <= 0x7e {
		return byte(r), true
	}

	switch r {
	case '\u00a0': // NBSP
		return 0xa0, true
	case '\u00b0': // °
		return 0xb0, true
	case '\u00b1': // ±
		return 0xb1, true
	case '\u00b7': // ·
		return 0xb7, true
	case '\u00a9': // ©
		return 0xa9, true
	case '\u00ae': // ®
		return 0xae, true
	case '\u2116': // №
		return 0xb9, true
	case '\u00ab': // «
		return 0xab, true
	case '\u00bb': // »
		return 0xbb, true
	case '\u2026': // …
		return 0x85, true
	case '\u2013': // –
		return 0x96, true
	case '\u2014': // —
		return 0x97, true
	case '\u0401': // Ё
		return 0xa8, true
	case '\u0451': // ё
		return 0xb8, true
	}

	if r >= '\u0410' && r <= '\u042f' { // А-Я
		return 0xc0 + byte(r-'\u0410'), true
	}
	if r >= '\u0430' && r <= '\u044f' { // а-я
		return 0xe0 + byte(r-'\u0430'), true
	}

	return 0, false
}
