package const2bitcolor

import (
	"image/color"

	"tinygo.org/x/drivers"
	"tinygo.org/x/tinyfont"
)

// Glyph is a 2bpp glyph.
type Glyph struct {
	Rune     rune
	Width    uint8
	Height   uint8
	XAdvance uint8
	XOffset  int8
	YOffset  int8
	Bitmaps  []byte
}

// Font is a compact 2bpp font backed by const strings.
//
// Concurrent access is not safe due to internal glyph reuse (matches tinyfont's const2bit behavior).
type Font struct {
	BBox     [4]int8 // width, height, minX, minY
	Glyphs   []Glyph
	YAdvance uint8

	OffsetMap string
	Data      string
	Name      string
	glyph     Glyph
}

func (glyph Glyph) Draw(display drivers.Displayer, x int16, y int16, c color.RGBA) {
	bitmapOffset := 0
	bitmap := byte(0)
	if len(glyph.Bitmaps) > 0 {
		bitmap = glyph.Bitmaps[bitmapOffset]
	}
	bit := uint8(0)
	for j := int16(0); j < int16(glyph.Height); j++ {
		for i := int16(0); i < int16(glyph.Width); i++ {
			switch bitmap & 0xC0 {
			case 0xC0:
				display.SetPixel(x+int16(glyph.XOffset)+i, y+int16(glyph.YOffset)+j, c)
			case 0x80:
				// Slightly boost mid-intensity pixels for crispness.
				display.SetPixel(x+int16(glyph.XOffset)+i, y+int16(glyph.YOffset)+j, scaleRGBA(c, 240))
			case 0x40:
				// Keep low-intensity pixels but dim them to avoid "mushy" antialiasing.
				display.SetPixel(x+int16(glyph.XOffset)+i, y+int16(glyph.YOffset)+j, scaleRGBA(c, 96))
			default:
			}

			bitmap <<= 2
			bit += 2
			if bit > 7 {
				bitmapOffset++
				if bitmapOffset < len(glyph.Bitmaps) {
					bitmap = glyph.Bitmaps[bitmapOffset]
				}
				bit = 0
			}
		}
	}
}

func (glyph Glyph) Info() tinyfont.GlyphInfo {
	return tinyfont.GlyphInfo{
		Rune:     glyph.Rune,
		Width:    glyph.Width,
		Height:   glyph.Height,
		XAdvance: glyph.XAdvance,
		XOffset:  glyph.XOffset,
		YOffset:  glyph.YOffset,
	}
}

func (f *Font) GetYAdvance() uint8 { return f.YAdvance }

func (font *Font) GetGlyph(r rune) tinyfont.Glypher {
	s := 0
	e := len(font.OffsetMap)/6 - 1

	for s <= e {
		m := (s + e) / 2

		r2 := rune(font.OffsetMap[m*6])<<16 + rune(font.OffsetMap[m*6+1])<<8 + rune(font.OffsetMap[m*6+2])
		if r2 < r {
			s = m + 1
		} else {
			e = m - 1
		}
	}

	if s > len(font.OffsetMap)/6-1 {
		s = 0
	}

	offset := uint32(font.OffsetMap[s*6+3])<<16 + uint32(font.OffsetMap[s*6+4])<<8 + uint32(font.OffsetMap[s*6+5])
	sz := uint32(len(font.Data[offset+5:]))
	if s*6+6 < len(font.OffsetMap) {
		sz = uint32(font.OffsetMap[s*6+9])<<16 + uint32(font.OffsetMap[s*6+10])<<8 + uint32(font.OffsetMap[s*6+11]) - offset
	}

	font.glyph.Rune = r
	font.glyph.Width = font.Data[offset+0]
	font.glyph.Height = font.Data[offset+1]
	font.glyph.XAdvance = font.Data[offset+2]
	font.glyph.XOffset = int8(font.Data[offset+3])
	font.glyph.YOffset = int8(font.Data[offset+4])
	font.glyph.Bitmaps = []byte(font.Data[offset+5 : offset+5+sz])
	return &(font.glyph)
}

// scaleRGBA scales RGB channels by factor/255, keeping alpha unchanged.
func scaleRGBA(c color.RGBA, factor uint8) color.RGBA {
	f := uint16(factor)
	return color.RGBA{
		R: uint8((uint16(c.R) * f) / 255),
		G: uint8((uint16(c.G) * f) / 255),
		B: uint8((uint16(c.B) * f) / 255),
		A: c.A,
	}
}
