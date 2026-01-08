package basic

import (
	"strings"
	"unicode"
)

type codeEditor struct {
	lines [][]rune

	curRow int
	curCol int

	top  int
	left int

	ghost string
	hint  string
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
	e.left = 0
	e.ghost = ""
	e.hint = ""
}

func (e *codeEditor) loadFromText(lines []string) {
	e.lines = nil
	for _, s := range lines {
		s = strings.TrimRight(s, "\r")
		e.lines = append(e.lines, []rune(s))
	}
	if len(e.lines) == 0 {
		e.lines = [][]rune{nil}
	}
	e.curRow = 0
	e.curCol = 0
	e.top = 0
	e.left = 0
	e.ghost = ""
	e.hint = ""
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
	if e.top < 0 {
		e.top = 0
	}
	if e.left < 0 {
		e.left = 0
	}
}

func (e *codeEditor) handleKey(k key, maxRows, maxCols int) {
	e.ensureNonEmpty()
	e.ghost = ""

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
			break
		}
		if e.curRow > 0 {
			e.curRow--
			e.curCol = len(e.lines[e.curRow])
		}
	case keyRight:
		if e.curCol < len(e.lines[e.curRow]) {
			e.curCol++
			break
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
		e.splitLine()
	case keyBackspace:
		e.backspace()
	case keyDelete:
		e.deleteForward()
	case keyTab:
		if e.ghost != "" {
			e.insertString(e.ghost)
		}
	case keyRune:
		if k.r >= 0x20 && k.r != 0x7f {
			e.insertRune(k.r)
		}
	}

	e.ensureVisible(maxRows, maxCols)
	e.updateHintAndGhost(maxCols)
}

func (e *codeEditor) splitLine() {
	line := e.lines[e.curRow]
	left := append([]rune(nil), line[:e.curCol]...)
	right := append([]rune(nil), line[e.curCol:]...)
	e.lines[e.curRow] = left
	e.lines = append(e.lines[:e.curRow+1], append([][]rune{right}, e.lines[e.curRow+1:]...)...)
	e.curRow++
	e.curCol = 0
}

func (e *codeEditor) backspace() {
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
}

func (e *codeEditor) deleteForward() {
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
}

func (e *codeEditor) insertRune(r rune) {
	line := e.lines[e.curRow]
	if e.curCol == len(line) {
		e.lines[e.curRow] = append(line, r)
		e.curCol++
		return
	}
	e.lines[e.curRow] = append(line[:e.curCol], append([]rune{r}, line[e.curCol:]...)...)
	e.curCol++
}

func (e *codeEditor) insertString(s string) {
	for _, r := range s {
		e.insertRune(r)
	}
}

func (e *codeEditor) ensureVisible(maxRows, maxCols int) {
	if maxRows <= 0 {
		maxRows = 1
	}
	if maxCols <= 0 {
		maxCols = 1
	}

	if e.curRow < e.top {
		e.top = e.curRow
	}
	if e.curRow >= e.top+maxRows {
		e.top = e.curRow - maxRows + 1
	}
	if e.top < 0 {
		e.top = 0
	}
	if e.top > len(e.lines)-1 {
		e.top = len(e.lines) - 1
	}

	if e.curCol < e.left {
		e.left = e.curCol
	}
	if e.curCol >= e.left+maxCols {
		e.left = e.curCol - maxCols + 1
	}
	if e.left < 0 {
		e.left = 0
	}
}

func (e *codeEditor) viewRowCount(maxRows int) int {
	if maxRows <= 0 {
		return 0
	}
	e.ensureNonEmpty()
	n := len(e.lines) - e.top
	if n < 0 {
		n = 0
	}
	if n > maxRows {
		n = maxRows
	}
	return n
}

func (e *codeEditor) lineRunes(row int) []rune {
	if row < 0 || row >= len(e.lines) {
		return nil
	}
	return e.lines[row]
}

func (e *codeEditor) cursorRune() rune {
	e.ensureNonEmpty()
	line := e.lines[e.curRow]
	if e.curCol < 0 || e.curCol >= len(line) {
		return ' '
	}
	return line[e.curCol]
}

func (e *codeEditor) updateHintAndGhost(maxCols int) {
	e.hint = ""
	e.ghost = ""
	word, prefix, ok := e.wordPrefixAtCursor()
	if !ok {
		return
	}
	if prefix == "" {
		return
	}

	up := strings.ToUpper(prefix)
	if h, ok := basicHints[up]; ok {
		e.hint = h
	}

	// Autocomplete only within reasonable viewport.
	_ = maxCols
	if word == "" {
		return
	}
	cands := completeKeyword(prefix)
	if len(cands) != 1 {
		return
	}
	cand := cands[0]
	if strings.EqualFold(cand, prefix) {
		return
	}
	if len(cand) <= len(prefix) {
		return
	}
	e.ghost = cand[len(prefix):]
}

func (e *codeEditor) wordPrefixAtCursor() (word string, prefix string, ok bool) {
	e.ensureNonEmpty()
	line := e.lines[e.curRow]
	if len(line) == 0 {
		return "", "", false
	}
	col := e.curCol
	if col > len(line) {
		col = len(line)
	}
	// Cursor is at insertion point; consider the rune before cursor for word.
	i := col
	if i > 0 && (i == len(line) || !isWordRune(line[i])) {
		i = i - 1
	}
	if i < 0 || i >= len(line) || !isWordRune(line[i]) {
		return "", "", false
	}
	start := i
	for start > 0 && isWordRune(line[start-1]) {
		start--
	}
	end := i + 1
	for end < len(line) && isWordRune(line[end]) {
		end++
	}
	word = string(line[start:end])
	// Prefix is up to cursor col.
	if col < start {
		return word, "", false
	}
	if col > end {
		col = end
	}
	prefix = string(line[start:col])
	return word, prefix, true
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

var basicKeywords = []string{
	"ABS", "AND", "CHR$", "CLS", "DEL", "DIM", "DIR", "ELSE",
	"COPY", "END", "EOF", "FOR", "GETB", "GETW", "GOSUB", "GOTO", "IF", "INPUT",
	"INT", "LEN", "LET", "NEXT", "OPEN", "OR", "POS", "PRINT", "PUTB", "PUTW",
	"REN", "REM", "RETURN", "RND", "RUN", "SEEK", "SGN", "SLEEP",
	"STOP", "THEN", "YIELD",
}

var basicHints = map[string]string{
	"PRINT":  "PRINT expr[, expr...]",
	"INPUT":  "INPUT A | INPUT A$",
	"IF":     "IF a <op> b THEN <stmt>",
	"FOR":    "FOR I = a TO b",
	"DIM":    "DIM A(n)",
	"GOTO":   "GOTO line",
	"GOSUB":  "GOSUB line",
	"RETURN": "RETURN",
	"RUN":    "RUN",
	"LIST":   "LIST",
	"NEW":    "NEW",
	"OPEN":   "OPEN fd, \"path\", \"R|W|A|RB|WB|AB\"",
	"GETB":   "GETB fd, A",
	"GETW":   "GETW fd, A",
	"PUTB":   "PUTB fd, expr",
	"PUTW":   "PUTW fd, expr",
	"SEEK":   "SEEK fd, pos",
	"POS":    "POS(fd)",
	"DIR":    "DIR \"path\"",
	"DEL":    "DEL \"path\"",
	"REN":    "REN \"old\",\"new\"",
	"COPY":   "COPY \"src\",\"dst\"",
	"EOF":    "EOF(fd)",
}

func completeKeyword(prefix string) []string {
	up := strings.ToUpper(prefix)
	var out []string
	for _, kw := range basicKeywords {
		if strings.HasPrefix(kw, up) {
			out = append(out, kw)
		}
	}
	return out
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
