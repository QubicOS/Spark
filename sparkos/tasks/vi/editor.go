//go:build spark_vi

package vi

import "fmt"

type mode uint8

const (
	modeNormal mode = iota
	modeInsert
	modeCmdline
	modeSearch
)

type keyKind uint8

const (
	keyRune keyKind = iota
	keyEnter
	keyBackspace
	keyTab
	keyEsc
	keyUp
	keyDown
	keyLeft
	keyRight
	keyDelete
	keyHome
	keyEnd
	keyCtrl
)

type key struct {
	kind keyKind
	r    rune
	ctrl byte
}

type snapshot struct {
	lines [][]rune

	cursorLine int
	cursorCol  int

	modified bool
	bytes    int
}

type editor struct {
	filePath string

	lines [][]rune

	cursorLine int
	cursorCol  int

	topLine int
	leftCol int

	mode mode

	pending rune

	cmdline []rune
	cmdPos  int

	search     []rune
	searchPos  int
	lastSearch []rune

	yankLine []rune

	message  string
	modified bool

	undo []snapshot
	redo []snapshot

	undoBytes int
	redoBytes int

	insertUndoPushed bool
}

const (
	maxUndoSnapshots = 64
	maxHistoryBytes  = 256 * 1024
)

func (e *editor) reset() {
	e.filePath = ""
	e.lines = [][]rune{{}}
	e.cursorLine = 0
	e.cursorCol = 0
	e.topLine = 0
	e.leftCol = 0
	e.mode = modeNormal
	e.pending = 0
	e.cmdline = nil
	e.cmdPos = 0
	e.search = nil
	e.searchPos = 0
	e.lastSearch = nil
	e.yankLine = nil
	e.message = ""
	e.modified = false
	e.undo = nil
	e.redo = nil
	e.undoBytes = 0
	e.redoBytes = 0
	e.insertUndoPushed = false
}

func (e *editor) setMessage(msg string) {
	e.message = msg
}

func (e *editor) ensureCursorVisible(viewRows, cols int) {
	e.clampCursor()
	if viewRows <= 0 || cols <= 0 {
		return
	}

	if e.cursorLine < e.topLine {
		e.topLine = e.cursorLine
	}
	if e.cursorLine >= e.topLine+viewRows {
		e.topLine = e.cursorLine - viewRows + 1
	}
	if e.topLine < 0 {
		e.topLine = 0
	}

	if e.cursorCol < e.leftCol {
		e.leftCol = e.cursorCol
	}
	if e.cursorCol >= e.leftCol+cols {
		e.leftCol = e.cursorCol - cols + 1
	}
	if e.leftCol < 0 {
		e.leftCol = 0
	}
}

func (e *editor) clampCursor() {
	if len(e.lines) == 0 {
		e.lines = [][]rune{{}}
	}
	if e.cursorLine < 0 {
		e.cursorLine = 0
	}
	if e.cursorLine >= len(e.lines) {
		e.cursorLine = len(e.lines) - 1
	}
	if e.cursorLine < 0 {
		e.cursorLine = 0
	}

	line := e.lines[e.cursorLine]
	if e.cursorCol < 0 {
		e.cursorCol = 0
	}
	if e.cursorCol > len(line) {
		e.cursorCol = len(line)
	}
}

func (e *editor) currentLine() []rune {
	if len(e.lines) == 0 {
		e.lines = [][]rune{{}}
		e.cursorLine = 0
		e.cursorCol = 0
	}
	if e.cursorLine < 0 {
		e.cursorLine = 0
	}
	if e.cursorLine >= len(e.lines) {
		e.cursorLine = len(e.lines) - 1
	}
	if e.cursorLine < 0 {
		e.cursorLine = 0
	}
	return e.lines[e.cursorLine]
}

func (e *editor) pushUndo() {
	b := snapshotBytes(e.lines)
	if b > maxHistoryBytes {
		e.undo = nil
		e.redo = nil
		e.undoBytes = 0
		e.redoBytes = 0
		return
	}

	cur := snapshot{
		lines:      cloneLines(e.lines),
		cursorLine: e.cursorLine,
		cursorCol:  e.cursorCol,
		modified:   e.modified,
		bytes:      b,
	}
	e.undo = append(e.undo, cur)
	e.undoBytes += cur.bytes
	for len(e.undo) > maxUndoSnapshots {
		e.undoBytes -= e.undo[0].bytes
		copy(e.undo, e.undo[1:])
		e.undo = e.undo[:len(e.undo)-1]
	}
	e.redo = nil
	e.redoBytes = 0
	e.trimHistory()
}

func (e *editor) undoOnce() {
	if len(e.undo) == 0 {
		e.setMessage("Already at oldest change.")
		return
	}

	b := snapshotBytes(e.lines)
	cur := snapshot{
		lines:      cloneLines(e.lines),
		cursorLine: e.cursorLine,
		cursorCol:  e.cursorCol,
		modified:   e.modified,
		bytes:      b,
	}
	if cur.bytes <= maxHistoryBytes {
		e.redo = append(e.redo, cur)
		e.redoBytes += cur.bytes
	}

	last := e.undo[len(e.undo)-1]
	e.undo = e.undo[:len(e.undo)-1]
	e.undoBytes -= last.bytes
	e.restore(last)
	e.trimHistory()
}

func (e *editor) redoOnce() {
	if len(e.redo) == 0 {
		e.setMessage("Already at newest change.")
		return
	}

	b := snapshotBytes(e.lines)
	cur := snapshot{
		lines:      cloneLines(e.lines),
		cursorLine: e.cursorLine,
		cursorCol:  e.cursorCol,
		modified:   e.modified,
		bytes:      b,
	}
	if cur.bytes <= maxHistoryBytes {
		e.undo = append(e.undo, cur)
		e.undoBytes += cur.bytes
		for len(e.undo) > maxUndoSnapshots {
			e.undoBytes -= e.undo[0].bytes
			copy(e.undo, e.undo[1:])
			e.undo = e.undo[:len(e.undo)-1]
		}
	}

	last := e.redo[len(e.redo)-1]
	e.redo = e.redo[:len(e.redo)-1]
	e.redoBytes -= last.bytes
	e.restore(last)
	e.trimHistory()
}

func (e *editor) restore(s snapshot) {
	e.lines = cloneLines(s.lines)
	e.cursorLine = s.cursorLine
	e.cursorCol = s.cursorCol
	e.modified = s.modified
	e.clampCursor()
}

func (e *editor) trimHistory() {
	for e.undoBytes+e.redoBytes > maxHistoryBytes {
		if len(e.redo) > 0 {
			e.redoBytes -= e.redo[0].bytes
			copy(e.redo, e.redo[1:])
			e.redo = e.redo[:len(e.redo)-1]
			continue
		}
		if len(e.undo) > 0 {
			e.undoBytes -= e.undo[0].bytes
			copy(e.undo, e.undo[1:])
			e.undo = e.undo[:len(e.undo)-1]
			continue
		}
		break
	}
}

func cloneLines(lines [][]rune) [][]rune {
	if len(lines) == 0 {
		return [][]rune{{}}
	}
	out := make([][]rune, len(lines))
	for i := range lines {
		out[i] = append([]rune(nil), lines[i]...)
	}
	return out
}

func snapshotBytes(lines [][]rune) int {
	var runes int
	for _, line := range lines {
		runes += len(line)
	}
	return runes * 4
}

func (e *editor) clearCmdline() {
	e.cmdline = e.cmdline[:0]
	e.cmdPos = 0
}

func (e *editor) takeCmdline() string {
	s := string(e.cmdline)
	e.clearCmdline()
	return s
}

func (e *editor) handleCmdlineKey(k key) {
	switch k.kind {
	case keyBackspace:
		if e.cmdPos <= 0 || len(e.cmdline) == 0 {
			return
		}
		copy(e.cmdline[e.cmdPos-1:], e.cmdline[e.cmdPos:])
		e.cmdline = e.cmdline[:len(e.cmdline)-1]
		e.cmdPos--
	case keyLeft:
		if e.cmdPos > 0 {
			e.cmdPos--
		}
	case keyRight:
		if e.cmdPos < len(e.cmdline) {
			e.cmdPos++
		}
	case keyHome:
		e.cmdPos = 0
	case keyEnd:
		e.cmdPos = len(e.cmdline)
	case keyRune:
		e.insertInto(&e.cmdline, &e.cmdPos, k.r)
	}
}

func (e *editor) clearSearch() {
	e.search = e.search[:0]
	e.searchPos = 0
}

func (e *editor) takeSearch() []rune {
	out := append([]rune(nil), e.search...)
	e.clearSearch()
	return out
}

func (e *editor) handleSearchKey(k key) {
	switch k.kind {
	case keyBackspace:
		if e.searchPos <= 0 || len(e.search) == 0 {
			return
		}
		copy(e.search[e.searchPos-1:], e.search[e.searchPos:])
		e.search = e.search[:len(e.search)-1]
		e.searchPos--
	case keyLeft:
		if e.searchPos > 0 {
			e.searchPos--
		}
	case keyRight:
		if e.searchPos < len(e.search) {
			e.searchPos++
		}
	case keyHome:
		e.searchPos = 0
	case keyEnd:
		e.searchPos = len(e.search)
	case keyRune:
		e.insertInto(&e.search, &e.searchPos, k.r)
	}
}

func (e *editor) insertInto(buf *[]rune, pos *int, r rune) {
	if *pos >= len(*buf) {
		*buf = append(*buf, r)
		*pos++
		return
	}
	*buf = append(*buf, 0)
	copy((*buf)[*pos+1:], (*buf)[*pos:])
	(*buf)[*pos] = r
	*pos++
}

func (e *editor) handleNormalKey(k key) (exit bool) {
	switch k.kind {
	case keyEsc:
		e.pending = 0
		return false
	case keyUp:
		e.moveUp()
		return false
	case keyDown:
		e.moveDown()
		return false
	case keyLeft:
		e.moveLeft()
		return false
	case keyRight:
		e.moveRight()
		return false
	case keyHome:
		e.cursorCol = 0
		return false
	case keyEnd:
		e.cursorCol = len(e.currentLine())
		return false
	case keyCtrl:
		switch k.ctrl {
		case 0x07:
			e.showStatus()
		case 0x12:
			e.redoOnce()
		}
		return false
	case keyRune:
	default:
		return false
	}

	r := k.r
	if e.pending != 0 {
		p := e.pending
		e.pending = 0
		return e.handlePending(p, r)
	}

	switch r {
	case 'h':
		e.moveLeft()
	case 'j':
		e.moveDown()
	case 'k':
		e.moveUp()
	case 'l':
		e.moveRight()
	case '0':
		e.cursorCol = 0
	case '$':
		e.cursorCol = len(e.currentLine())
	case 'G':
		e.cursorLine = len(e.lines) - 1
		e.cursorCol = 0
	case 'i':
		e.pushUndo()
		e.insertUndoPushed = true
		e.mode = modeInsert
	case 'a':
		e.pushUndo()
		e.insertUndoPushed = true
		if e.cursorCol < len(e.currentLine()) {
			e.cursorCol++
		}
		e.mode = modeInsert
	case 'o':
		e.pushUndo()
		e.openLineBelow()
		e.insertUndoPushed = true
		e.mode = modeInsert
	case 'O':
		e.pushUndo()
		e.openLineAbove()
		e.insertUndoPushed = true
		e.mode = modeInsert
	case 'x':
		e.pushUndo()
		e.deleteChar()
	case 'd', 'y', 'g':
		e.pending = r
	case 'p':
		e.pushUndo()
		e.pasteBelow()
	case 'P':
		e.pushUndo()
		e.pasteAbove()
	case 'u':
		e.undoOnce()
	case ':':
		e.mode = modeCmdline
		e.clearCmdline()
	case '/':
		e.mode = modeSearch
		e.clearSearch()
	case 'n':
		e.searchForward(e.lastSearch)
	}

	return false
}

func (e *editor) handlePending(p, r rune) bool {
	switch p {
	case 'd':
		switch r {
		case 'd':
			e.pushUndo()
			e.deleteLine()
		default:
			e.setMessage("Unsupported delete command.")
		}
	case 'y':
		switch r {
		case 'y':
			e.yankLine = append([]rune(nil), e.currentLine()...)
			e.setMessage("1 line yanked.")
		default:
			e.setMessage("Unsupported yank command.")
		}
	case 'g':
		if r == 'g' {
			e.cursorLine = 0
			e.cursorCol = 0
		}
	}
	return false
}

func (e *editor) handleInsertKey(k key) (exit bool) {
	switch k.kind {
	case keyEsc:
		e.mode = modeNormal
		e.pending = 0
		e.insertUndoPushed = false
		if e.cursorCol > 0 {
			e.cursorCol--
		}
		return false
	case keyEnter:
		e.modified = true
		e.insertNewline()
		return false
	case keyBackspace:
		e.modified = true
		e.backspace()
		return false
	case keyDelete:
		e.modified = true
		e.deleteChar()
		return false
	case keyUp:
		e.moveUp()
		return false
	case keyDown:
		e.moveDown()
		return false
	case keyLeft:
		e.moveLeft()
		return false
	case keyRight:
		e.moveRight()
		return false
	case keyTab:
		e.modified = true
		e.insertRune('\t')
		return false
	case keyCtrl:
		switch k.ctrl {
		case 0x03:
			e.mode = modeNormal
			e.pending = 0
			e.insertUndoPushed = false
			if e.cursorCol > 0 {
				e.cursorCol--
			}
		}
		return false
	case keyRune:
		e.modified = true
		e.insertRune(k.r)
		return false
	default:
		return false
	}
}

func (e *editor) showStatus() {
	name := e.filePath
	if name == "" {
		name = "[No Name]"
	}
	mod := ""
	if e.modified {
		mod = " [modified]"
	}
	e.setMessage(fmt.Sprintf("\"%s\" %d lines%s", name, len(e.lines), mod))
}

func (e *editor) moveLeft() {
	if e.cursorCol > 0 {
		e.cursorCol--
		return
	}
	if e.cursorLine <= 0 {
		return
	}
	e.cursorLine--
	e.cursorCol = len(e.currentLine())
}

func (e *editor) moveRight() {
	line := e.currentLine()
	if e.cursorCol < len(line) {
		e.cursorCol++
		return
	}
	if e.cursorLine >= len(e.lines)-1 {
		return
	}
	e.cursorLine++
	e.cursorCol = 0
}

func (e *editor) moveUp() {
	if e.cursorLine <= 0 {
		return
	}
	e.cursorLine--
	line := e.currentLine()
	if e.cursorCol > len(line) {
		e.cursorCol = len(line)
	}
}

func (e *editor) moveDown() {
	if e.cursorLine >= len(e.lines)-1 {
		return
	}
	e.cursorLine++
	line := e.currentLine()
	if e.cursorCol > len(line) {
		e.cursorCol = len(line)
	}
}

func (e *editor) insertRune(r rune) {
	e.modified = true
	line := e.currentLine()
	i := e.cursorCol
	line = append(line, 0)
	copy(line[i+1:], line[i:])
	line[i] = r
	e.lines[e.cursorLine] = line
	e.cursorCol++
}

func (e *editor) backspace() {
	if e.cursorCol > 0 {
		e.modified = true
		line := e.currentLine()
		i := e.cursorCol - 1
		copy(line[i:], line[i+1:])
		line = line[:len(line)-1]
		e.lines[e.cursorLine] = line
		e.cursorCol--
		return
	}
	if e.cursorLine <= 0 {
		return
	}

	e.modified = true
	cur := append([]rune(nil), e.currentLine()...)
	e.lines = append(e.lines[:e.cursorLine], e.lines[e.cursorLine+1:]...)
	e.cursorLine--
	prev := e.currentLine()
	e.cursorCol = len(prev)
	prev = append(prev, cur...)
	e.lines[e.cursorLine] = prev
}

func (e *editor) insertNewline() {
	e.modified = true
	line := e.currentLine()
	i := e.cursorCol
	left := append([]rune(nil), line[:i]...)
	right := append([]rune(nil), line[i:]...)
	e.lines[e.cursorLine] = left
	e.lines = append(e.lines, nil)
	copy(e.lines[e.cursorLine+2:], e.lines[e.cursorLine+1:])
	e.lines[e.cursorLine+1] = right
	e.cursorLine++
	e.cursorCol = 0
}

func (e *editor) deleteChar() {
	line := e.currentLine()
	if e.cursorCol >= len(line) {
		return
	}
	e.modified = true
	i := e.cursorCol
	copy(line[i:], line[i+1:])
	line = line[:len(line)-1]
	e.lines[e.cursorLine] = line
}

func (e *editor) deleteLine() {
	e.modified = true
	if len(e.lines) == 0 {
		e.lines = [][]rune{{}}
		e.cursorLine = 0
		e.cursorCol = 0
		return
	}
	e.lines = append(e.lines[:e.cursorLine], e.lines[e.cursorLine+1:]...)
	if len(e.lines) == 0 {
		e.lines = [][]rune{{}}
		e.cursorLine = 0
		e.cursorCol = 0
		return
	}
	if e.cursorLine >= len(e.lines) {
		e.cursorLine = len(e.lines) - 1
	}
	e.cursorCol = 0
}

func (e *editor) openLineBelow() {
	e.modified = true
	e.cursorLine++
	if e.cursorLine > len(e.lines) {
		e.cursorLine = len(e.lines)
	}
	e.lines = append(e.lines, nil)
	copy(e.lines[e.cursorLine+1:], e.lines[e.cursorLine:])
	e.lines[e.cursorLine] = []rune{}
	e.cursorCol = 0
}

func (e *editor) openLineAbove() {
	e.modified = true
	e.lines = append(e.lines, nil)
	copy(e.lines[e.cursorLine+1:], e.lines[e.cursorLine:])
	e.lines[e.cursorLine] = []rune{}
	e.cursorCol = 0
}

func (e *editor) pasteBelow() {
	if e.yankLine == nil {
		return
	}
	i := e.cursorLine + 1
	if i > len(e.lines) {
		i = len(e.lines)
	}
	e.lines = append(e.lines, nil)
	copy(e.lines[i+1:], e.lines[i:])
	e.lines[i] = append([]rune(nil), e.yankLine...)
	e.cursorLine = i
	e.cursorCol = 0
	e.modified = true
}

func (e *editor) pasteAbove() {
	if e.yankLine == nil {
		return
	}
	i := e.cursorLine
	e.lines = append(e.lines, nil)
	copy(e.lines[i+1:], e.lines[i:])
	e.lines[i] = append([]rune(nil), e.yankLine...)
	e.cursorLine = i
	e.cursorCol = 0
	e.modified = true
}

func (e *editor) searchForward(pat []rune) {
	if len(pat) == 0 {
		e.setMessage("No previous regular expression.")
		return
	}
	e.lastSearch = append(e.lastSearch[:0], pat...)

	startLine := e.cursorLine
	startCol := e.cursorCol + 1
	for li := startLine; li < len(e.lines); li++ {
		from := 0
		if li == startLine {
			from = startCol
		}
		pos := indexRunes(e.lines[li], pat, from)
		if pos >= 0 {
			e.cursorLine = li
			e.cursorCol = pos
			return
		}
	}
	e.setMessage("Pattern not found.")
}

func indexRunes(hay, needle []rune, start int) int {
	if start < 0 {
		start = 0
	}
	if len(needle) == 0 {
		return -1
	}
	if start > len(hay) {
		return -1
	}
	for i := start; i+len(needle) <= len(hay); i++ {
		ok := true
		for j := range needle {
			if hay[i+j] != needle[j] {
				ok = false
				break
			}
		}
		if ok {
			return i
		}
	}
	return -1
}
