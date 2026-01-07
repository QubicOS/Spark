package basic

import (
	"image/color"
	"strings"
	"unicode"

	"tinygo.org/x/tinyfont"
)

var keywordSet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(basicKeywords))
	for _, kw := range basicKeywords {
		m[kw] = struct{}{}
	}
	return m
}()

func drawBasicLine(
	d *fbDisplay,
	font tinyfont.Fonter,
	fontWidth int16,
	x, y int16,
	line []rune,
	left, cols int,
) {
	if cols <= 0 {
		return
	}
	if left < 0 {
		left = 0
	}

	segments := tokenizeBasic(line)
	for _, seg := range segments {
		segStart := seg.start
		segEnd := seg.end

		if segEnd <= left {
			continue
		}
		if segStart >= left+cols {
			break
		}

		start := segStart
		if start < left {
			start = left
		}
		end := segEnd
		if end > left+cols {
			end = left + cols
		}
		if start >= end {
			continue
		}

		s := string(line[start:end])
		px := x + int16(start-left)*fontWidth
		tinyfont.WriteLine(d, font, px, y, s, seg.c)
	}
}

type span struct {
	start int
	end   int
	c     color.RGBA
}

func tokenizeBasic(line []rune) []span {
	if len(line) == 0 {
		return nil
	}
	var out []span

	emit := func(start, end int, c color.RGBA) {
		if start >= end {
			return
		}
		out = append(out, span{start: start, end: end, c: c})
	}

	// Default color for everything.
	emit(0, len(line), colorFG)

	// Highlight line number (leading digits after optional spaces).
	i := 0
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	j := i
	for j < len(line) && unicode.IsDigit(line[j]) {
		j++
	}
	if j > i {
		emit(i, j, colorDim)
	}

	// Strings, keywords, and REM comments.
	inString := false
	rem := false

	for pos := 0; pos < len(line); pos++ {
		if rem {
			emit(pos, len(line), colorDim)
			break
		}
		r := line[pos]
		if inString {
			if r == '"' {
				inString = false
				emit(pos, pos+1, colorInputFG)
				continue
			}
			emit(pos, pos+1, colorInputFG)
			continue
		}
		if r == '"' {
			inString = true
			emit(pos, pos+1, colorInputFG)
			continue
		}

		if isWordRune(r) {
			start := pos
			for pos < len(line) && isWordRune(line[pos]) {
				pos++
			}
			word := strings.ToUpper(string(line[start:pos]))
			if word == "REM" {
				emit(start, pos, colorAccent)
				rem = true
				pos--
				continue
			}
			if _, ok := keywordSet[word]; ok {
				emit(start, pos, colorAccent)
			}
			pos--
			continue
		}
	}
	return out
}
