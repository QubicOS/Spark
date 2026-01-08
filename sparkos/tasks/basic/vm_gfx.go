package basic

import (
	"image/color"

	"tinygo.org/x/tinyfont"
)

func (m *vm) execCLS(_ *scanner) (stepResult, error) {
	if m.fb == nil {
		return stepResult{}, nil
	}
	m.fb.ClearRGB(0, 0, 0)
	_ = m.fb.Present()
	return stepResult{}, nil
}

func (m *vm) execPSet(s *scanner) (stepResult, error) {
	if m.fb == nil || m.d == nil {
		return stepResult{}, nil
	}
	x, y, c, err := parseXYC(m, s)
	if err != nil {
		return stepResult{}, err
	}
	m.d.SetPixel(int16(x), int16(y), colorFromInt(c))
	_ = m.fb.Present()
	return stepResult{}, nil
}

func (m *vm) execGfxLine(s *scanner) (stepResult, error) {
	if m.fb == nil || m.d == nil {
		return stepResult{}, nil
	}
	x1, y1, x2, y2, c, err := parseLineArgs(m, s)
	if err != nil {
		return stepResult{}, err
	}
	drawLine(m.d, x1, y1, x2, y2, colorFromInt(c))
	_ = m.fb.Present()
	return stepResult{}, nil
}

func (m *vm) execRect(s *scanner) (stepResult, error) {
	if m.fb == nil || m.d == nil {
		return stepResult{}, nil
	}
	x, y, w, h, c, err := parseRectArgs(m, s)
	if err != nil {
		return stepResult{}, err
	}
	col := colorFromInt(c)
	drawRect(m.d, x, y, w, h, col)
	_ = m.fb.Present()
	return stepResult{}, nil
}

func (m *vm) execText(s *scanner) (stepResult, error) {
	if m.fb == nil || m.d == nil || m.font == nil {
		return stepResult{}, nil
	}
	x, err := parseIntExpr(m, s)
	if err != nil {
		return stepResult{}, err
	}
	s.skipSpaces()
	if !s.accept(',') {
		return stepResult{}, ErrSyntax
	}
	y, err := parseIntExpr(m, s)
	if err != nil {
		return stepResult{}, err
	}
	s.skipSpaces()
	if !s.accept(',') {
		return stepResult{}, ErrSyntax
	}
	str, err := m.parseStringExpr(s)
	if err != nil {
		return stepResult{}, err
	}
	tinyfont.WriteLine(m.d, m.font, int16(x), int16(y)+m.fontOffset, str, colorFG)
	_ = m.fb.Present()
	return stepResult{}, nil
}

func parseXYC(m *vm, s *scanner) (x, y, c int32, err error) {
	x, err = parseIntExpr(m, s)
	if err != nil {
		return 0, 0, 0, err
	}
	s.skipSpaces()
	if !s.accept(',') {
		return 0, 0, 0, ErrSyntax
	}
	y, err = parseIntExpr(m, s)
	if err != nil {
		return 0, 0, 0, err
	}
	s.skipSpaces()
	if !s.accept(',') {
		return 0, 0, 0, ErrSyntax
	}
	c, err = parseIntExpr(m, s)
	if err != nil {
		return 0, 0, 0, err
	}
	return x, y, c, nil
}

func parseLineArgs(m *vm, s *scanner) (x1, y1, x2, y2, c int32, err error) {
	x1, err = parseIntExpr(m, s)
	if err != nil {
		return 0, 0, 0, 0, 0, err
	}
	s.skipSpaces()
	if !s.accept(',') {
		return 0, 0, 0, 0, 0, ErrSyntax
	}
	y1, err = parseIntExpr(m, s)
	if err != nil {
		return 0, 0, 0, 0, 0, err
	}
	s.skipSpaces()
	if !s.accept(',') {
		return 0, 0, 0, 0, 0, ErrSyntax
	}
	x2, err = parseIntExpr(m, s)
	if err != nil {
		return 0, 0, 0, 0, 0, err
	}
	s.skipSpaces()
	if !s.accept(',') {
		return 0, 0, 0, 0, 0, ErrSyntax
	}
	y2, err = parseIntExpr(m, s)
	if err != nil {
		return 0, 0, 0, 0, 0, err
	}
	s.skipSpaces()
	if !s.accept(',') {
		return 0, 0, 0, 0, 0, ErrSyntax
	}
	c, err = parseIntExpr(m, s)
	if err != nil {
		return 0, 0, 0, 0, 0, err
	}
	return x1, y1, x2, y2, c, nil
}

func parseRectArgs(m *vm, s *scanner) (x, y, w, h, c int32, err error) {
	x, err = parseIntExpr(m, s)
	if err != nil {
		return 0, 0, 0, 0, 0, err
	}
	s.skipSpaces()
	if !s.accept(',') {
		return 0, 0, 0, 0, 0, ErrSyntax
	}
	y, err = parseIntExpr(m, s)
	if err != nil {
		return 0, 0, 0, 0, 0, err
	}
	s.skipSpaces()
	if !s.accept(',') {
		return 0, 0, 0, 0, 0, ErrSyntax
	}
	w, err = parseIntExpr(m, s)
	if err != nil {
		return 0, 0, 0, 0, 0, err
	}
	s.skipSpaces()
	if !s.accept(',') {
		return 0, 0, 0, 0, 0, ErrSyntax
	}
	h, err = parseIntExpr(m, s)
	if err != nil {
		return 0, 0, 0, 0, 0, err
	}
	s.skipSpaces()
	if !s.accept(',') {
		return 0, 0, 0, 0, 0, ErrSyntax
	}
	c, err = parseIntExpr(m, s)
	if err != nil {
		return 0, 0, 0, 0, 0, err
	}
	return x, y, w, h, c, nil
}

func colorFromInt(v int32) color.RGBA {
	if v < 0 {
		v = 0
	}
	// Palette 0..15.
	switch v {
	case 0:
		return colorBG
	case 1:
		return colorFG
	case 2:
		return colorAccent
	case 3:
		return colorNum
	case 4:
		return colorString
	case 5:
		return colorVar
	case 6:
		return colorOp
	case 7:
		return colorComment
	case 8:
		return colorDim
	case 9:
		return colorInputFG
	}
	// Grayscale fallback.
	gray := uint8(v & 0xff)
	return color.RGBA{R: gray, G: gray, B: gray, A: 0xff}
}

func drawLine(d *fbDisplay, x1, y1, x2, y2 int32, c color.RGBA) {
	ix1 := int(x1)
	iy1 := int(y1)
	ix2 := int(x2)
	iy2 := int(y2)

	dx := absInt(ix2 - ix1)
	sx := -1
	if ix1 < ix2 {
		sx = 1
	}
	dy := -absInt(iy2 - iy1)
	sy := -1
	if iy1 < iy2 {
		sy = 1
	}
	err := dx + dy
	for {
		d.SetPixel(int16(ix1), int16(iy1), c)
		if ix1 == ix2 && iy1 == iy2 {
			return
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			ix1 += sx
		}
		if e2 <= dx {
			err += dx
			iy1 += sy
		}
	}
}

func drawRect(d *fbDisplay, x, y, w, h int32, c color.RGBA) {
	if w < 0 {
		x += w
		w = -w
	}
	if h < 0 {
		y += h
		h = -h
	}
	x0 := x
	y0 := y
	x1 := x + w - 1
	y1 := y + h - 1
	drawLine(d, x0, y0, x1, y0, c)
	drawLine(d, x0, y1, x1, y1, c)
	drawLine(d, x0, y0, x0, y1, c)
	drawLine(d, x1, y0, x1, y1, c)
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
