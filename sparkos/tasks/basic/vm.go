package basic

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"spark/hal"
	vfsclient "spark/sparkos/client/vfs"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"

	"tinygo.org/x/tinyfont"
)

var (
	ErrSyntax     = errors.New("syntax error")
	ErrType       = errors.New("type mismatch")
	ErrBadLine    = errors.New("bad line number")
	ErrBadFile    = errors.New("bad file descriptor")
	ErrBadSub     = errors.New("return without gosub")
	ErrBadNext    = errors.New("next without for")
	ErrBadDim     = errors.New("bad dim")
	ErrBadCommand = errors.New("unknown command")
)

type varKind uint8

const (
	varInt varKind = iota + 1
	varString
	varArray
)

type varRef struct {
	kind  varKind
	index int
	sub   int
}

type stepResult struct {
	output     []string
	halt       bool
	awaitInput bool
	awaitVar   varRef
}

type forFrame struct {
	varIndex int
	end      int32
	pc       int
}

type fileMode uint8

const (
	fileRead fileMode = iota + 1
	fileWrite
	fileAppend
)

type fileHandle struct {
	inUse bool
	mode  fileMode
	path  string
	pos   uint32
	w     *vfsclient.Writer
}

type vm struct {
	ctx  *kernel.Context
	vfs  *vfsclient.Client
	prog *program
	fb   hal.Framebuffer
	d    *fbDisplay

	running bool
	pc      int
	index   map[int]int

	intVars [26]int32
	strVars [26]string
	arrVars [26][]int32

	callStack []int
	forStack  []forFrame

	files []fileHandle

	pendingInputs []varRef

	font       tinyfont.Fonter
	fontWidth  int16
	fontHeight int16
	fontOffset int16

	spawnFn func(path string) error
}

func newVM(maxFiles int) *vm {
	if maxFiles <= 0 {
		maxFiles = defaultMaxFiles
	}
	return &vm{files: make([]fileHandle, maxFiles)}
}

func (m *vm) reset() {
	m.running = false
	m.pc = 0
	m.index = nil
	m.intVars = [26]int32{}
	m.strVars = [26]string{}
	m.arrVars = [26][]int32{}
	m.callStack = nil
	m.forStack = nil
	for i := range m.files {
		m.files[i] = fileHandle{}
	}
	m.d = nil
	m.font = nil
	m.fontWidth = 0
	m.fontHeight = 0
	m.fontOffset = 0
	m.spawnFn = nil
}

func (m *vm) start() error {
	if m.prog == nil {
		return fmt.Errorf("run: %w", ErrBadCommand)
	}
	if m.fb != nil && m.d == nil {
		m.d = newFBDisplay(m.fb)
	}
	m.index = make(map[int]int, len(m.prog.lines))
	for i, ln := range m.prog.lines {
		m.index[ln.no] = i
	}
	m.pc = 0
	m.running = true
	return nil
}

func (m *vm) execImmediate(line string) (stepResult, error) {
	if m.ctx == nil {
		return stepResult{}, errors.New("immediate: missing ctx")
	}
	return m.execLine(line)
}

func (m *vm) step() (stepResult, error) {
	if !m.running || m.prog == nil {
		return stepResult{halt: true}, nil
	}
	if m.pc < 0 || m.pc >= len(m.prog.lines) {
		return stepResult{halt: true}, nil
	}
	ln := m.prog.lines[m.pc]
	res, err := m.execLine(ln.text)
	if err != nil {
		return stepResult{}, fmt.Errorf("line %d: %w", ln.no, err)
	}
	if res.halt || res.awaitInput {
		return res, nil
	}
	if m.running {
		m.pc++
	}
	return res, nil
}

func (m *vm) execLine(text string) (stepResult, error) {
	s := newScanner(text)
	s.skipSpaces()
	if s.eof() {
		return stepResult{}, nil
	}

	word := strings.ToUpper(s.readWord())
	switch word {
	case "REM":
		return stepResult{}, nil
	case "LET":
		return m.execLet(&s)
	case "PRINT":
		return m.execPrint(&s)
	case "INPUT":
		return m.execInput(&s)
	case "IF":
		return m.execIf(&s)
	case "GOTO":
		return m.execGoto(&s)
	case "GOSUB":
		return m.execGosub(&s)
	case "RETURN":
		return m.execReturn(&s)
	case "FOR":
		return m.execFor(&s)
	case "NEXT":
		return m.execNext(&s)
	case "END", "STOP":
		return stepResult{halt: true}, nil
	case "SLEEP":
		return m.execSleep(&s)
	case "YIELD":
		m.ctx.BlockOnTick()
		return stepResult{}, nil
	case "DIM":
		return m.execDim(&s)

	case "OPEN":
		return m.execOpen(&s)
	case "CLOSE":
		return m.execClose(&s)
	case "GETB":
		return m.execGetB(&s)
	case "PUTB":
		return m.execPutB(&s)
	case "SEEK":
		return m.execSeek(&s)
	case "DIR":
		return m.execDir(&s)
	case "MKDIR":
		return m.execMkdir(&s)
	case "RMDIR":
		return m.execRmdir(&s)
	case "STAT":
		return m.execStat(&s)
	case "DEL":
		return m.execDel(&s)
	case "REN":
		return m.execRen(&s)
	case "COPY":
		return m.execCopy(&s)
	case "GETW":
		return m.execGetW(&s)
	case "PUTW":
		return m.execPutW(&s)
	case "SPAWN":
		return m.execSpawn(&s)
	case "CLS":
		return m.execCLS(&s)
	case "PSET":
		return m.execPSet(&s)
	case "LINE":
		return m.execGfxLine(&s)
	case "RECT":
		return m.execRect(&s)
	case "TEXT":
		return m.execText(&s)
	default:
		s = newScanner(text)
		return m.execAssignment(&s)
	}
}

func (m *vm) setFromInput(v varRef, raw string) error {
	raw = strings.TrimSpace(raw)
	switch v.kind {
	case varInt, varArray:
		n, err := strconv.ParseInt(raw, 10, 32)
		if err != nil {
			return ErrType
		}
		return m.setVar(v, int32(n))
	case varString:
		return m.setVar(v, raw)
	default:
		return ErrType
	}
}

func (m *vm) setVar(v varRef, val any) error {
	switch v.kind {
	case varInt:
		n, ok := val.(int32)
		if !ok {
			return ErrType
		}
		m.intVars[v.index] = n
		return nil
	case varString:
		s, ok := val.(string)
		if !ok {
			return ErrType
		}
		m.strVars[v.index] = s
		return nil
	case varArray:
		n, ok := val.(int32)
		if !ok {
			return ErrType
		}
		arr := m.arrVars[v.index]
		if v.sub < 0 || v.sub >= len(arr) {
			return ErrBadDim
		}
		arr[v.sub] = n
		return nil
	default:
		return ErrType
	}
}

func (m *vm) getVar(v varRef) (any, error) {
	switch v.kind {
	case varInt:
		return m.intVars[v.index], nil
	case varString:
		return m.strVars[v.index], nil
	case varArray:
		arr := m.arrVars[v.index]
		if v.sub < 0 || v.sub >= len(arr) {
			return int32(0), ErrBadDim
		}
		return arr[v.sub], nil
	default:
		return nil, ErrType
	}
}

func (m *vm) gotoLine(lineNo int32) error {
	if m.index == nil {
		return ErrBadLine
	}
	i, ok := m.index[int(lineNo)]
	if !ok {
		return ErrBadLine
	}
	m.pc = i - 1
	return nil
}

func (m *vm) rebuildIndex() {
	if m.prog == nil {
		m.index = nil
		return
	}
	m.index = make(map[int]int, len(m.prog.lines))
	for i, ln := range m.prog.lines {
		m.index[ln.no] = i
	}
}

func (m *vm) getFile(fd int32) (*fileHandle, error) {
	if fd <= 0 || int(fd) > len(m.files) {
		return nil, ErrBadFile
	}
	h := &m.files[int(fd)-1]
	if !h.inUse {
		return nil, ErrBadFile
	}
	return h, nil
}

func (m *vm) ensureVFS() error {
	if m.vfs == nil || m.ctx == nil {
		return errors.New("vfs: not available")
	}
	return nil
}

func vfsMode(m fileMode) (proto.VFSWriteMode, error) {
	switch m {
	case fileWrite:
		return proto.VFSWriteTruncate, nil
	case fileAppend:
		return proto.VFSWriteAppend, nil
	default:
		return 0, ErrBadFile
	}
}
