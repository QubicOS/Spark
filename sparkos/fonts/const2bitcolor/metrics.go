package const2bitcolor

import (
	"errors"
	"fmt"
)

// ComputeLineMetrics derives font cell metrics for tinyterm-like renderers.
//
// It returns:
//   - fontHeight: total cell height in pixels
//   - fontOffset: baseline offset from the top of the cell
//
// The computation scans glyph headers in the font's OffsetMap/Data.
func ComputeLineMetrics(font *Font) (fontHeight int16, fontOffset int16, err error) {
	if font == nil {
		return 0, 0, errors.New("nil font")
	}
	if len(font.OffsetMap)%6 != 0 {
		return 0, 0, errors.New("bad offset map")
	}
	if len(font.Data) < 5 {
		return 0, 0, errors.New("bad data")
	}

	entries := len(font.OffsetMap) / 6
	minY := 0
	maxY := 0
	first := true

	for i := 0; i < entries; i++ {
		base := i * 6
		off := int(uint32(font.OffsetMap[base+3])<<16 | uint32(font.OffsetMap[base+4])<<8 | uint32(font.OffsetMap[base+5]))
		if off < 0 || off+5 >= len(font.Data) {
			continue
		}

		h := int(font.Data[off+1])
		yoff := int(int8(font.Data[off+4]))
		if first {
			minY = yoff
			maxY = yoff + h
			first = false
			continue
		}
		if yoff < minY {
			minY = yoff
		}
		if yoff+h > maxY {
			maxY = yoff + h
		}
	}
	if first {
		return 0, 0, errors.New("no glyphs")
	}

	height := maxY - minY
	offset := -minY
	if height <= 0 || offset < 0 {
		return 0, 0, fmt.Errorf("invalid metrics: height=%d offset=%d", height, offset)
	}
	if height > 127 || offset > 127 {
		return 0, 0, fmt.Errorf("metrics too large: height=%d offset=%d", height, offset)
	}
	return int16(height), int16(offset), nil
}

// ComputeTerminalMetrics returns compact per-line metrics for terminal-style rendering.
//
// It uses font.YAdvance as the line height and picks a baseline offset that minimizes
// clipping at both the top and bottom based on the font's glyph extents.
func ComputeTerminalMetrics(font *Font) (fontHeight int16, fontOffset int16, err error) {
	bboxHeight, bboxOffset, err := ComputeLineMetrics(font)
	if err != nil {
		return 0, 0, err
	}

	h := int16(font.YAdvance)
	if h <= 0 {
		h = bboxHeight
	}

	minY := -bboxOffset
	maxY := bboxHeight - bboxOffset

	// Choose offset to balance top/bottom clipping for the chosen line height.
	off := (h - maxY - minY) / 2
	if off < 0 {
		off = 0
	}
	if off > h {
		off = h
	}
	return h, off, nil
}
