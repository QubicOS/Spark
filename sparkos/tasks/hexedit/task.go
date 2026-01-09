package hexedit

import (
	"bytes"
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

const (
	maxSearchBytes = 2 * 1024 * 1024
	maxVFSRead     = kernel.MaxMessageBytes - 11
)

type inputKind uint8

const (
	inputNone inputKind = iota
	inputGoto
	inputFindASCII
	inputFindHex
)

// Task implements a standalone hex viewer/editor.
type Task struct {
	disp hal.Display
	ep   kernel.Capability

	vfsCap kernel.Capability
	vfs    *vfsclient.Client

	fb hal.Framebuffer
	d  *fbDisplay

	font       tinyfont.Fonter
	fontWidth  int16
	fontHeight int16
	fontOffset int16

	cols     int
	rows     int
	viewRows int

	active bool
	muxCap kernel.Capability

	path     string
	origSize uint32
	size     uint32

	data []byte
	mods map[uint32]byte

	winOff  uint32
	winData []byte

	cursor uint32
	topRow int

	editASCII bool
	viewASCII bool
	nibble    uint8
	dirty     bool
	quitAsk   bool

	message string

	showHelp bool
	helpTop  int

	inbuf []byte

	inMode  inputKind
	inLabel string
	inLine  []rune

	lastPattern []byte
	lastIsHex   bool
	lastFound   uint32
	lastOK      bool
}

func New(disp hal.Display, ep kernel.Capability, vfsCap kernel.Capability) *Task {
	return &Task{disp: disp, ep: ep, vfsCap: vfsCap}
}

func (t *Task) Run(ctx *kernel.Context) {
	ch, ok := ctx.RecvChan(t.ep)
	if !ok {
		return
	}
	if t.disp == nil {
		return
	}

	t.fb = t.disp.Framebuffer()
	if t.fb == nil {
		return
	}
	t.d = newFBDisplay(t.fb)

	if !t.initFont() {
		return
	}

	t.cols = t.fb.Width() / int(t.fontWidth)
	t.rows = t.fb.Height() / int(t.fontHeight)
	t.viewRows = t.rows - 2
	if t.cols <= 0 || t.rows <= 0 || t.viewRows <= 0 {
		return
	}

	for msg := range ch {
		switch proto.Kind(msg.Kind) {
		case proto.MsgAppShutdown:
			t.unloadSession()
			return

		case proto.MsgAppControl:
			if msg.Cap.Valid() {
				t.muxCap = msg.Cap
			}
			active, ok := proto.DecodeAppControlPayload(msg.Payload())
			if !ok {
				continue
			}
			t.setActive(ctx, active)

		case proto.MsgAppSelect:
			appID, arg, ok := proto.DecodeAppSelectPayload(msg.Payload())
			if !ok || appID != proto.AppHex {
				continue
			}
			if arg != "" {
				if err := t.loadFile(ctx, arg); err != nil {
					t.setMessage("open: " + err.Error())
				}
			}
			if t.active {
				t.render(ctx)
			}

		case proto.MsgTermInput:
			if !t.active {
				continue
			}
			t.handleInput(ctx, msg.Payload())
			if t.active {
				t.render(ctx)
			}
		}
	}
}

func (t *Task) setActive(ctx *kernel.Context, active bool) {
	if active == t.active {
		if !active {
			t.unloadSession()
		}
		return
	}
	t.active = active
	if !t.active {
		t.unloadSession()
		return
	}
	t.setMessage("H help | g goto | / find | v view | i edit | w save | q quit")
	t.render(ctx)
}

func (t *Task) setMessage(msg string) {
	t.message = msg
}

func (t *Task) vfsClient() *vfsclient.Client {
	if t.vfs == nil {
		t.vfs = vfsclient.New(t.vfsCap)
	}
	return t.vfs
}

func (t *Task) requestExit(ctx *kernel.Context) {
	if t.fb != nil {
		t.fb.ClearRGB(0, 0, 0)
		_ = t.fb.Present()
	}
	t.unloadSession()

	if !t.muxCap.Valid() {
		return
	}
	for {
		res := ctx.SendToCapResult(t.muxCap, uint16(proto.MsgAppControl), proto.AppControlPayload(false), kernel.Capability{})
		switch res {
		case kernel.SendOK:
			return
		case kernel.SendErrQueueFull:
			ctx.BlockOnTick()
		default:
			return
		}
	}
}

func (t *Task) unloadSession() {
	t.active = false
	t.vfs = nil

	t.path = ""
	t.origSize = 0
	t.size = 0

	t.data = nil
	t.mods = nil
	t.winOff = 0
	t.winData = nil

	t.cursor = 0
	t.topRow = 0

	t.editASCII = false
	t.viewASCII = false
	t.nibble = 0
	t.dirty = false
	t.quitAsk = false

	t.message = ""

	t.showHelp = false
	t.helpTop = 0

	t.inbuf = nil

	t.inMode = inputNone
	t.inLabel = ""
	t.inLine = nil

	t.lastPattern = nil
	t.lastIsHex = false
	t.lastFound = 0
	t.lastOK = false
}

func (t *Task) handleInput(ctx *kernel.Context, b []byte) {
	t.inbuf = append(t.inbuf, b...)
	buf := t.inbuf

	for len(buf) > 0 {
		n, k, ok := nextKey(buf)
		if !ok {
			break
		}
		buf = buf[n:]

		t.handleKey(ctx, k)
		if !t.active {
			t.inbuf = t.inbuf[:0]
			return
		}
	}

	t.inbuf = append(t.inbuf[:0], buf...)
}

func (t *Task) handleKey(ctx *kernel.Context, k key) {
	if k.kind == keyRune && (k.r == 'H' || k.r == 'h') {
		t.showHelp = !t.showHelp
		if t.showHelp {
			t.helpTop = 0
		}
		return
	}
	if t.showHelp {
		t.handleHelpKey(k)
		return
	}

	if t.inMode != inputNone {
		t.handleInputModeKey(ctx, k)
		return
	}

	if k.kind != keyRune || (k.r != 'w' && k.r != 'q') {
		t.quitAsk = false
	}

	switch k.kind {
	case keyEsc:
		t.handleQuit(ctx)
	case keyEnter:
		// No-op.
	case keyUp:
		t.moveRel(-int64(t.layoutBytesPerRow()))
	case keyDown:
		t.moveRel(int64(t.layoutBytesPerRow()))
	case keyLeft:
		t.moveRel(-1)
	case keyRight:
		t.moveRel(1)
	case keyHome:
		t.cursor = 0
		t.nibble = 0
	case keyEnd:
		if t.size > 0 {
			t.cursor = t.size - 1
		}
		t.nibble = 0
	case keyPageUp:
		t.page(-1)
	case keyPageDown:
		t.page(1)
	case keyRune:
		switch k.r {
		case 'q':
			t.handleQuit(ctx)
		case 'i':
			t.editASCII = !t.editASCII
			if t.editASCII {
				t.setMessage("edit: ASCII")
			} else {
				t.setMessage("edit: HEX")
			}
		case 'v':
			t.viewASCII = !t.viewASCII
			if t.viewASCII {
				t.setMessage("view: ASCII")
			} else {
				t.setMessage("view: HEX")
			}
		case 'g':
			t.beginInput(inputGoto, "goto (hex/dec): ")
		case '/':
			t.beginInput(inputFindASCII, "find ASCII: ")
		case '?':
			t.beginInput(inputFindHex, "find HEX bytes (e.g. DE AD BE EF): ")
		case 'n':
			t.findNext(ctx, true)
		case 'N':
			t.findPrev(ctx)
		case 'w':
			t.save(ctx)
		default:
			t.editRune(ctx, k.r)
		}
	}

	t.ensureVisible(t.layoutBytesPerRow())
}

func (t *Task) handleHelpKey(k key) {
	switch k.kind {
	case keyEsc, keyEnter:
		t.showHelp = false
	case keyUp:
		if t.helpTop > 0 {
			t.helpTop--
		}
	case keyDown:
		t.helpTop++
	case keyHome:
		t.helpTop = 0
	case keyEnd:
		t.helpTop = 1 << 30
	}
}

func (t *Task) handleQuit(ctx *kernel.Context) {
	if t.dirty && !t.quitAsk {
		t.quitAsk = true
		t.setMessage("unsaved: w save | q discard")
		return
	}
	t.requestExit(ctx)
}

func (t *Task) page(dir int) {
	step := int64(t.layoutBytesPerRow() * t.viewRows)
	if step <= 0 {
		step = 16
	}
	t.moveRel(int64(dir) * step)
}

func (t *Task) moveRel(delta int64) {
	if t.size == 0 {
		t.cursor = 0
		return
	}
	n := int64(t.cursor) + delta
	if n < 0 {
		n = 0
	}
	if n >= int64(t.size) {
		n = int64(t.size - 1)
	}
	t.cursor = uint32(n)
	t.nibble = 0
}

func (t *Task) beginInput(kind inputKind, label string) {
	t.inMode = kind
	t.inLabel = label
	t.inLine = t.inLine[:0]
}

func (t *Task) handleInputModeKey(ctx *kernel.Context, k key) {
	switch k.kind {
	case keyEsc:
		t.inMode = inputNone
		t.inLabel = ""
		t.inLine = t.inLine[:0]
		return
	case keyEnter:
		line := string(t.inLine)
		mode := t.inMode
		t.inMode = inputNone
		t.inLabel = ""
		t.inLine = t.inLine[:0]
		t.finishInput(ctx, mode, line)
		return
	case keyBackspace:
		if len(t.inLine) > 0 {
			t.inLine = t.inLine[:len(t.inLine)-1]
		}
		return
	case keyRune:
		if k.r < 0x20 {
			return
		}
		if len(t.inLine) >= 64 {
			return
		}
		t.inLine = append(t.inLine, k.r)
		return
	default:
		return
	}
}

func (t *Task) finishInput(ctx *kernel.Context, mode inputKind, line string) {
	line = strings.TrimSpace(line)
	switch mode {
	case inputGoto:
		if line == "" {
			return
		}
		off, err := parseOffset(line)
		if err != nil {
			t.setMessage("goto: " + err.Error())
			return
		}
		if t.size == 0 {
			t.cursor = 0
			return
		}
		if off >= t.size {
			off = t.size - 1
		}
		t.cursor = off
		t.nibble = 0
		t.ensureVisible(t.layoutBytesPerRow())
		t.setMessage("goto " + fmt.Sprintf("0x%X", off))
	case inputFindASCII:
		if line == "" {
			return
		}
		t.lastPattern = []byte(line)
		t.lastIsHex = false
		t.findNext(ctx, false)
	case inputFindHex:
		if line == "" {
			return
		}
		pat, err := parseHexPattern(line)
		if err != nil {
			t.setMessage("find: " + err.Error())
			return
		}
		if len(pat) == 0 {
			return
		}
		t.lastPattern = pat
		t.lastIsHex = true
		t.findNext(ctx, false)
	default:
	}
}

func parseOffset(s string) (uint32, error) {
	base := 10
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		base = 16
		s = s[2:]
	}
	if s == "" {
		return 0, errors.New("empty offset")
	}
	u, err := strconv.ParseUint(s, base, 32)
	if err != nil {
		return 0, errors.New("invalid offset")
	}
	return uint32(u), nil
}

func parseHexPattern(s string) ([]byte, error) {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return nil, errors.New("empty pattern")
	}
	out := make([]byte, 0, len(fields))
	for _, f := range fields {
		f = strings.TrimPrefix(strings.TrimPrefix(f, "0x"), "0X")
		if len(f) == 0 || len(f) > 2 {
			return nil, errors.New("bad byte: " + f)
		}
		u, err := strconv.ParseUint(f, 16, 8)
		if err != nil {
			return nil, errors.New("bad byte: " + f)
		}
		out = append(out, byte(u))
	}
	return out, nil
}

func (t *Task) findNext(ctx *kernel.Context, fromCursor bool) {
	if len(t.lastPattern) == 0 || t.path == "" {
		return
	}
	start := uint32(0)
	if fromCursor {
		start = t.cursor + 1
	} else if t.lastOK {
		start = t.lastFound + 1
	} else {
		start = t.cursor
	}

	pos, ok, err := t.searchForward(ctx, start, t.lastPattern)
	if err != nil {
		t.setMessage("find: " + err.Error())
		return
	}
	if !ok {
		t.lastOK = false
		t.setMessage("find: not found")
		return
	}
	t.cursor = pos
	t.nibble = 0
	t.lastFound = pos
	t.lastOK = true
	t.ensureVisible(t.layoutBytesPerRow())
	t.setMessage("found at " + fmt.Sprintf("0x%X", pos))
}

func (t *Task) findPrev(ctx *kernel.Context) {
	if len(t.lastPattern) == 0 || t.path == "" {
		return
	}
	if t.cursor == 0 {
		t.setMessage("find: not found")
		return
	}

	target := t.cursor - 1
	pos, ok, err := t.searchPrev(ctx, target, t.lastPattern)
	if err != nil {
		t.setMessage("find: " + err.Error())
		return
	}
	if !ok {
		t.lastOK = false
		t.setMessage("find: not found")
		return
	}
	t.cursor = pos
	t.nibble = 0
	t.lastFound = pos
	t.lastOK = true
	t.ensureVisible(t.layoutBytesPerRow())
	t.setMessage("found at " + fmt.Sprintf("0x%X", pos))
}

func (t *Task) searchForward(ctx *kernel.Context, start uint32, pat []byte) (uint32, bool, error) {
	if len(pat) == 0 || t.size == 0 || start >= t.size {
		return 0, false, nil
	}
	if t.size > maxSearchBytes {
		return 0, false, fmt.Errorf("file too large for search (>%d bytes)", maxSearchBytes)
	}

	var buf []byte
	var off uint32 = start
	tail := make([]byte, 0, len(pat)-1)
	for off < t.size {
		chunk, eof, err := t.vfsClient().ReadAt(ctx, t.path, off, maxVFSRead)
		if err != nil {
			return 0, false, err
		}
		if len(chunk) == 0 && eof {
			return 0, false, nil
		}

		buf = buf[:0]
		buf = append(buf, tail...)
		buf = append(buf, chunk...)
		t.applyMods(off-uint32(len(tail)), buf)

		if idx := bytes.Index(buf, pat); idx >= 0 {
			return (off - uint32(len(tail))) + uint32(idx), true, nil
		}

		if len(pat) > 1 {
			if len(buf) >= len(pat)-1 {
				tail = append(tail[:0], buf[len(buf)-(len(pat)-1):]...)
			} else {
				tail = append(tail[:0], buf...)
			}
		}

		off += uint32(len(chunk))
	}
	return 0, false, nil
}

func (t *Task) searchPrev(ctx *kernel.Context, end uint32, pat []byte) (uint32, bool, error) {
	if len(pat) == 0 || t.size == 0 {
		return 0, false, nil
	}
	if t.size > maxSearchBytes {
		return 0, false, fmt.Errorf("file too large for search (>%d bytes)", maxSearchBytes)
	}

	var last uint32
	ok := false
	var start uint32
	for {
		pos, found, err := t.searchForward(ctx, start, pat)
		if err != nil {
			return 0, false, err
		}
		if !found {
			return last, ok, nil
		}
		if pos > end {
			return last, ok, nil
		}
		last = pos
		ok = true
		start = pos + 1
	}
}

func (t *Task) applyMods(chunkOff uint32, chunk []byte) {
	if len(chunk) == 0 || len(t.mods) == 0 {
		return
	}
	for i := range chunk {
		off := chunkOff + uint32(i)
		if b, ok := t.mods[off]; ok {
			chunk[i] = b
		}
	}
}

func (t *Task) editRune(ctx *kernel.Context, r rune) {
	if t.size == 0 {
		return
	}
	if t.editASCII {
		if r < 0x20 || r > 0x7e {
			return
		}
		t.setByte(ctx, t.cursor, byte(r))
		t.moveRel(1)
		return
	}

	val, ok := hexDigitValue(r)
	if !ok {
		return
	}
	prev, ok := t.byteAt(ctx, t.cursor)
	if !ok {
		return
	}
	next := prev
	if t.nibble == 0 {
		next = (prev & 0x0f) | (val << 4)
		t.nibble = 1
	} else {
		next = (prev & 0xf0) | val
		t.nibble = 0
	}
	if next != prev {
		t.setByte(ctx, t.cursor, next)
	}
	if t.nibble == 0 {
		t.moveRel(1)
	}
}

func hexDigitValue(r rune) (byte, bool) {
	switch {
	case r >= '0' && r <= '9':
		return byte(r - '0'), true
	case r >= 'a' && r <= 'f':
		return byte(r-'a') + 10, true
	case r >= 'A' && r <= 'F':
		return byte(r-'A') + 10, true
	default:
		return 0, false
	}
}

func (t *Task) loadFile(ctx *kernel.Context, path string) error {
	typ, size, err := t.vfsClient().Stat(ctx, path)
	if err != nil {
		return err
	}
	if typ != proto.VFSEntryFile {
		return errors.New("not a file")
	}

	t.path = path
	t.origSize = size
	t.size = size
	t.cursor = 0
	t.topRow = 0
	t.editASCII = false
	t.nibble = 0
	t.dirty = false
	t.quitAsk = false
	t.message = ""
	t.mods = nil
	t.winOff = 0
	t.winData = t.winData[:0]
	t.lastPattern = nil
	t.lastOK = false

	if size == 0 {
		t.data = nil
		return nil
	}
	t.data = nil
	return nil
}

func (t *Task) setByte(ctx *kernel.Context, off uint32, b byte) {
	prev, ok := t.byteAt(ctx, off)
	if ok && prev == b {
		return
	}
	if t.data != nil {
		if off >= uint32(len(t.data)) {
			return
		}
		t.data[off] = b
	} else {
		if t.mods == nil {
			t.mods = make(map[uint32]byte)
		}
		t.mods[off] = b
		if off >= t.size {
			t.size = off + 1
		}
	}

	t.dirty = true
	t.quitAsk = false
}

func (t *Task) byteAt(ctx *kernel.Context, off uint32) (byte, bool) {
	if off >= t.size {
		return 0, false
	}
	if t.data != nil {
		if off >= uint32(len(t.data)) {
			return 0, false
		}
		return t.data[off], true
	}
	if t.mods != nil {
		if b, ok := t.mods[off]; ok {
			return b, true
		}
	}
	b, ok := t.readWindowByte(ctx, off)
	return b, ok
}

func (t *Task) readWindowByte(ctx *kernel.Context, off uint32) (byte, bool) {
	if off >= t.origSize {
		return 0, false
	}
	winSize := int(maxVFSRead)
	if winSize <= 0 {
		winSize = 256
	}

	if off < t.winOff || off >= t.winOff+uint32(len(t.winData)) {
		base := off &^ 0xfff
		chunk, _, err := t.vfsClient().ReadAt(ctx, t.path, base, uint16(winSize))
		if err != nil || len(chunk) == 0 {
			return 0, false
		}
		t.winOff = base
		t.winData = append(t.winData[:0], chunk...)
		t.applyMods(base, t.winData)
	}

	i := off - t.winOff
	if i >= uint32(len(t.winData)) {
		return 0, false
	}
	return t.winData[i], true
}

func (t *Task) layoutBytesPerRow() int {
	if t.viewASCII {
		return t.layoutASCIIBytesPerRow()
	}
	n, _ := t.layout()
	return n
}

func (t *Task) layout() (bytesPerRow int, showASCII bool) {
	cols := t.cols
	if cols <= 0 {
		return 16, false
	}
	for n := 16; n >= 4; n-- {
		hexCols := 10 + n*3
		if hexCols <= cols {
			if hexCols+1+n <= cols {
				return n, true
			}
			return n, false
		}
	}
	return 4, false
}

func (t *Task) layoutASCIIBytesPerRow() int {
	n := t.cols - 10
	if n > 64 {
		n = 64
	}
	if n < 4 {
		n = 4
	}
	return n
}

func (t *Task) ensureVisible(bytesPerRow int) {
	if bytesPerRow <= 0 || t.viewRows <= 0 || t.size == 0 {
		t.topRow = 0
		return
	}

	row := int(t.cursor) / bytesPerRow
	if row < 0 {
		row = 0
	}
	if row < t.topRow {
		t.topRow = row
	} else if row >= t.topRow+t.viewRows {
		t.topRow = row - (t.viewRows - 1)
	}

	maxTop := int((t.size-1)/uint32(bytesPerRow)) - (t.viewRows - 1)
	if maxTop < 0 {
		maxTop = 0
	}
	if t.topRow < 0 {
		t.topRow = 0
	}
	if t.topRow > maxTop {
		t.topRow = maxTop
	}
}

func (t *Task) save(ctx *kernel.Context) {
	if t.path == "" {
		return
	}
	if !t.dirty {
		t.setMessage("saved")
		return
	}

	var data []byte
	if t.data != nil {
		data = t.data
	} else {
		data = nil
	}

	if data != nil {
		if _, err := t.vfsClient().Write(ctx, t.path, proto.VFSWriteTruncate, data); err != nil {
			t.setMessage("save: " + err.Error())
			return
		}
		t.dirty = false
		t.quitAsk = false
		t.origSize = uint32(len(data))
		t.size = t.origSize
		t.mods = nil
		t.setMessage("saved")
		return
	}

	if t.size == 0 {
		if _, err := t.vfsClient().Write(ctx, t.path, proto.VFSWriteTruncate, nil); err != nil {
			t.setMessage("save: " + err.Error())
			return
		}
		t.dirty = false
		t.quitAsk = false
		t.origSize = 0
		t.mods = nil
		t.setMessage("saved")
		return
	}

	w, err := t.vfsClient().OpenWriter(ctx, t.path, proto.VFSWriteTruncate)
	if err != nil {
		t.setMessage("save: " + err.Error())
		return
	}

	chunkSize := int(maxVFSRead)
	if chunkSize <= 0 {
		chunkSize = 256
	}
	buf := make([]byte, 0, chunkSize)
	var off uint32
	for off < t.size {
		want := uint32(chunkSize)
		if off+want > t.size {
			want = t.size - off
		}

		buf = buf[:0]
		if off < t.origSize {
			chunk, _, rerr := t.vfsClient().ReadAt(ctx, t.path, off, uint16(want))
			if rerr != nil {
				_, _ = w.Close()
				t.setMessage("save: " + rerr.Error())
				return
			}
			buf = append(buf, chunk...)
			if uint32(len(buf)) < want {
				buf = append(buf, make([]byte, int(want-uint32(len(buf))))...)
			}
		} else {
			buf = append(buf, make([]byte, int(want))...)
		}

		t.applyMods(off, buf)
		if _, werr := w.Write(buf); werr != nil {
			_, _ = w.Close()
			t.setMessage("save: " + werr.Error())
			return
		}
		off += want
	}

	if _, err := w.Close(); err != nil {
		t.setMessage("save: " + err.Error())
		return
	}

	t.dirty = false
	t.quitAsk = false
	t.origSize = t.size
	t.mods = nil
	t.setMessage("saved")
}

func (t *Task) statusText() string {
	if t.inMode != inputNone {
		return clipRunes(t.inLabel+string(t.inLine), t.cols)
	}

	edit := "HEX"
	if t.editASCII {
		edit = "ASCII"
	}
	view := "HEX"
	if t.viewASCII {
		view = "ASCII"
	}
	nib := ""
	if !t.editASCII {
		if t.nibble == 0 {
			nib = " hi"
		} else {
			nib = " lo"
		}
	}

	flags := ""
	if t.dirty {
		flags = "*"
	}
	base := fmt.Sprintf(
		"%soff=%08X size=%08X view=%s edit=%s%s | g goto | / find | v view | i edit | w save | q quit",
		flags,
		t.cursor,
		t.size,
		view,
		edit,
		nib,
	)
	if t.message == "" {
		return clipRunes(base, t.cols)
	}
	return clipRunes(t.message+" | "+base, t.cols)
}

func clipRunes(s string, max int) string {
	rs := []rune(s)
	if len(rs) > max {
		rs = rs[:max]
	}
	return string(rs)
}
