package basic

import (
	"fmt"
	"strings"
)

type codeEditor struct {
	lines [][]rune

	curRow int
	curCol int

	top int
}

func (e *codeEditor) loadFromProgram(p *program) {
	e.lines = nil
	for _, ln := range p.lines {
		e.lines = append(e.lines, []rune(ln.String()))
	}
	if len(e.lines) == 0 {
		e.lines = [][]rune{nil}
	}
	e.curRow = 0
	e.curCol = 0
	e.top = 0
}

func (e *codeEditor) ensureNonEmpty() {
	if len(e.lines) == 0 {
		e.lines = [][]rune{nil}
	}
	if e.curRow < 0 {
		e.curRow = 0
	}
	if e.curRow >= len(e.lines) {
		e.curRow = len(e.lines) - 1
	}
	if e.curCol < 0 {
		e.curCol = 0
	}
	if e.curCol > len(e.lines[e.curRow]) {
		e.curCol = len(e.lines[e.curRow])
	}
}

func (e *codeEditor) handleKey(k key, cols int) {
	e.ensureNonEmpty()
	switch k.kind {
	case keyUp:
		if e.curRow > 0 {
			e.curRow--
		}
		if e.curCol > len(e.lines[e.curRow]) {
			e.curCol = len(e.lines[e.curRow])
		}
	case keyDown:
		if e.curRow < len(e.lines)-1 {
			e.curRow++
		}
		if e.curCol > len(e.lines[e.curRow]) {
			e.curCol = len(e.lines[e.curRow])
		}
	case keyLeft:
		if e.curCol > 0 {
			e.curCol--
			return
		}
		if e.curRow > 0 {
			e.curRow--
			e.curCol = len(e.lines[e.curRow])
		}
	case keyRight:
		if e.curCol < len(e.lines[e.curRow]) {
			e.curCol++
			return
		}
		if e.curRow < len(e.lines)-1 {
			e.curRow++
			e.curCol = 0
		}
	case keyHome:
		e.curCol = 0
	case keyEnd:
		e.curCol = len(e.lines[e.curRow])
	case keyEnter:
		line := e.lines[e.curRow]
		left := append([]rune(nil), line[:e.curCol]...)
		right := append([]rune(nil), line[e.curCol:]...)
		e.lines[e.curRow] = left
		e.lines = append(e.lines[:e.curRow+1], append([][]rune{right}, e.lines[e.curRow+1:]...)...)
		e.curRow++
		e.curCol = 0
	case keyBackspace:
		if e.curCol > 0 {
			line := e.lines[e.curRow]
			e.lines[e.curRow] = append(line[:e.curCol-1], line[e.curCol:]...)
			e.curCol--
			return
		}
		if e.curRow == 0 {
			return
		}
		prev := e.lines[e.curRow-1]
		cur := e.lines[e.curRow]
		e.curCol = len(prev)
		e.lines[e.curRow-1] = append(prev, cur...)
		copy(e.lines[e.curRow:], e.lines[e.curRow+1:])
		e.lines = e.lines[:len(e.lines)-1]
		e.curRow--
	case keyDelete:
		line := e.lines[e.curRow]
		if e.curCol < len(line) {
			e.lines[e.curRow] = append(line[:e.curCol], line[e.curCol+1:]...)
			return
		}
		if e.curRow >= len(e.lines)-1 {
			return
		}
		e.lines[e.curRow] = append(e.lines[e.curRow], e.lines[e.curRow+1]...)
		copy(e.lines[e.curRow+1:], e.lines[e.curRow+2:])
		e.lines = e.lines[:len(e.lines)-1]
	case keyRune:
		if k.r == 0 {
			return
		}
		line := e.lines[e.curRow]
		if e.curCol == len(line) {
			e.lines[e.curRow] = append(line, k.r)
			e.curCol++
			return
		}
		e.lines[e.curRow] = append(line[:e.curCol], append([]rune{k.r}, line[e.curCol:]...)...)
		e.curCol++
	}

	e.ensureNonEmpty()
	e.ensureVisible(cols)
}

func (e *codeEditor) ensureVisible(_ int) {
	if e.top > e.curRow {
		e.top = e.curRow
		return
	}
}

func (e *codeEditor) visible(maxRows int) (out []string, top int) {
	e.ensureNonEmpty()
	if maxRows <= 0 {
		return nil, 0
	}
	top = e.top
	if top < 0 {
		top = 0
	}
	if top > e.curRow {
		top = e.curRow
	}
	if top >= len(e.lines) {
		top = len(e.lines) - 1
	}
	if top < 0 {
		top = 0
	}

	// Clamp top so cursor fits in view.
	if e.curRow >= top+maxRows {
		top = e.curRow - maxRows + 1
	}
	if top < 0 {
		top = 0
	}
	if top >= len(e.lines) {
		top = len(e.lines) - 1
	}

	end := top + maxRows
	if end > len(e.lines) {
		end = len(e.lines)
	}

	for i := top; i < end; i++ {
		s := string(e.lines[i])
		if i == e.curRow {
			s = insertCaretRunes(e.lines[i], e.curCol)
		}
		out = append(out, s)
	}
	e.top = top
	return out, top
}

func insertCaretRunes(r []rune, cur int) string {
	if cur < 0 {
		cur = 0
	}
	if cur > len(r) {
		cur = len(r)
	}
	out := make([]rune, 0, len(r)+1)
	out = append(out, r[:cur]...)
	out = append(out, '|')
	out = append(out, r[cur:]...)
	return string(out)
}

func (e *codeEditor) toLines() []string {
	e.ensureNonEmpty()
	out := make([]string, 0, len(e.lines))
	for _, r := range e.lines {
		s := strings.TrimRight(string(r), " \t")
		out = append(out, s)
	}
	return out
}

func (e *codeEditor) debugCursor() string {
	e.ensureNonEmpty()
	return fmt.Sprintf("row=%d col=%d top=%d", e.curRow, e.curCol, e.top)
}
