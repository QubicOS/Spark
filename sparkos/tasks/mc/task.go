package mc

import (
	"errors"
	"fmt"
	"strings"

	"spark/hal"
	vfsclient "spark/sparkos/client/vfs"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"

	"tinygo.org/x/tinyfont"
)

type appMode uint8

const (
	modePanels appMode = iota
	modeViewer
	modeHex
)

type panelID uint8

const (
	panelLeft panelID = iota
	panelRight
)

const (
	maxViewerBytes = 64 * 1024
	maxHexBytes    = 128 * 1024
	maxCopyBytes   = 1024 * 1024
	maxVFSRead     = kernel.MaxMessageBytes - 11
)

// Task implements a Midnight Commander-like file manager.
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

	cols      int
	rows      int
	viewRows  int
	panelCols int

	active bool
	muxCap kernel.Capability

	mode        appMode
	activePanel panelID

	left  panel
	right panel

	message string

	inbuf []byte

	showHelp bool
	helpTop  int

	viewerPath  string
	viewerLines [][]rune
	viewerTop   int

	hexPath        string
	hexData        []byte
	hexCursor      int
	hexTop         int
	hexNibble      uint8
	hexDirty       bool
	hexQuitConfirm bool
	hexEditASCII   bool
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
	t.panelCols = (t.cols - 1) / 2
	if t.cols <= 0 || t.rows <= 0 || t.viewRows <= 0 || t.panelCols <= 0 {
		return
	}

	t.activePanel = panelLeft
	t.left.setPath("/")
	t.right.setPath("/")

	for msg := range ch {
		switch proto.Kind(msg.Kind) {
		case proto.MsgAppControl:
			if msg.Cap.Valid() {
				t.muxCap = msg.Cap
			}
			active, ok := proto.DecodeAppControlPayload(msg.Data[:msg.Len])
			if !ok {
				continue
			}
			t.setActive(ctx, active)

		case proto.MsgAppSelect:
			appID, arg, ok := proto.DecodeAppSelectPayload(msg.Data[:msg.Len])
			if !ok || appID != proto.AppMC {
				continue
			}
			if arg != "" {
				t.left.setPath(arg)
				t.right.setPath(arg)
			}
			if t.active {
				_ = t.refreshPanels(ctx)
				t.render()
			}

		case proto.MsgTermInput:
			if !t.active {
				continue
			}
			t.handleInput(ctx, msg.Data[:msg.Len])
			if t.active {
				t.render()
			}
		}
	}
}

func (t *Task) setActive(ctx *kernel.Context, active bool) {
	if active == t.active {
		return
	}
	t.active = active
	if !t.active {
		return
	}
	_ = t.refreshPanels(ctx)
	t.setMessage("H help | TAB switch | ENTER open | q quit")
	t.render()
}

func (t *Task) setMessage(msg string) {
	t.message = msg
}

func (t *Task) requestExit(ctx *kernel.Context) {
	if t.fb != nil {
		t.fb.ClearRGB(0, 0, 0)
		_ = t.fb.Present()
	}

	t.active = false
	t.showHelp = false

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
			t.active = false
			return
		}
	}
}

func (t *Task) activePanelPtr() *panel {
	if t.activePanel == panelRight {
		return &t.right
	}
	return &t.left
}

func (t *Task) otherPanelPtr() *panel {
	if t.activePanel == panelRight {
		return &t.left
	}
	return &t.right
}

func (t *Task) refreshPanels(ctx *kernel.Context) error {
	if err := t.loadDir(ctx, &t.left); err != nil {
		t.setMessage("left: " + err.Error())
		return err
	}
	if err := t.loadDir(ctx, &t.right); err != nil {
		t.setMessage("right: " + err.Error())
		return err
	}
	t.left.clamp(t.viewRows)
	t.right.clamp(t.viewRows)
	return nil
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

	switch t.mode {
	case modeViewer:
		t.handleViewerKey(ctx, k)
		return
	case modeHex:
		t.handleHexKey(ctx, k)
		return
	default:
	}

	switch k.kind {
	case keyEsc:
		t.requestExit(ctx)
	case keyTab:
		if t.activePanel == panelLeft {
			t.activePanel = panelRight
		} else {
			t.activePanel = panelLeft
		}
	case keyLeft, keyRight:
		if k.kind == keyLeft {
			t.activePanel = panelLeft
		} else {
			t.activePanel = panelRight
		}
	case keyUp:
		t.activePanelPtr().up()
	case keyDown:
		t.activePanelPtr().down()
	case keyHome:
		t.activePanelPtr().sel = 0
	case keyEnd:
		p := t.activePanelPtr()
		p.sel = len(p.entries) - 1
	case keyBackspace:
		t.goParent(ctx)
	case keyEnter:
		t.openSelected(ctx)
	case keyCtrl:
		if k.ctrl == 0x12 {
			_ = t.refreshPanels(ctx)
		}
	case keyRune:
		switch k.r {
		case 'q':
			t.requestExit(ctx)
		case 'r':
			_ = t.refreshPanels(ctx)
		case 'c':
			if err := t.copySelected(ctx); err != nil {
				t.setMessage("copy: " + err.Error())
			}
		case 'n':
			if err := t.mkdirAuto(ctx); err != nil {
				t.setMessage("mkdir: " + err.Error())
			}
		case 'x':
			if err := t.openHexSelected(ctx); err != nil {
				t.setMessage("hex: " + err.Error())
			}
		}
	}

	t.activePanelPtr().clamp(t.viewRows)
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
	case keyRune:
		switch k.r {
		case 'q':
			t.showHelp = false
		}
	}
}

func (t *Task) goParent(ctx *kernel.Context) {
	p := t.activePanelPtr()
	if p.path == "/" {
		return
	}
	if err := t.cd(ctx, p, ".."); err != nil {
		t.setMessage(err.Error())
	}
}

func (t *Task) openSelected(ctx *kernel.Context) {
	p := t.activePanelPtr()
	e, ok := p.selected()
	if !ok {
		return
	}
	if e.isDir() {
		if err := t.cd(ctx, p, e.Name); err != nil {
			t.setMessage(err.Error())
		}
		return
	}

	if err := t.openViewer(ctx, e.FullPath); err != nil {
		t.setMessage("view: " + err.Error())
		return
	}
}

func (t *Task) openHexSelected(ctx *kernel.Context) error {
	p := t.activePanelPtr()
	e, ok := p.selected()
	if !ok {
		return errors.New("no selection")
	}
	if e.isDir() || e.Name == ".." {
		return errors.New("select a file")
	}
	return t.openHex(ctx, e.FullPath)
}

func (t *Task) cd(ctx *kernel.Context, p *panel, name string) error {
	var target string
	if name == ".." {
		if p.path == "/" {
			target = "/"
		} else {
			target = cleanPath(pathDir(p.path))
		}
	} else {
		target = joinPath(p.path, name)
	}

	typ, _, err := t.vfsClient().Stat(ctx, target)
	if err != nil {
		return err
	}
	if typ != proto.VFSEntryDir {
		return errors.New("not a directory")
	}
	p.setPath(target)
	p.sel = 0
	p.scroll = 0
	if err := t.loadDir(ctx, p); err != nil {
		return err
	}
	p.clamp(t.viewRows)
	return nil
}

func pathDir(p string) string {
	last := -1
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			last = i
			break
		}
	}
	if last <= 0 {
		return "/"
	}
	return p[:last]
}

func (t *Task) mkdirAuto(ctx *kernel.Context) error {
	p := t.activePanelPtr()
	base := "newdir"
	for i := 0; i < 100; i++ {
		name := base
		if i > 0 {
			name = fmt.Sprintf("%s%d", base, i)
		}
		target := joinPath(p.path, name)
		if err := t.vfsClient().Mkdir(ctx, target); err == nil {
			_ = t.loadDir(ctx, p)
			t.setMessage("created " + target)
			return nil
		}
	}
	return errors.New("failed to pick name")
}

func (t *Task) copySelected(ctx *kernel.Context) error {
	srcPanel := t.activePanelPtr()
	dstPanel := t.otherPanelPtr()

	e, ok := srcPanel.selected()
	if !ok {
		return errors.New("no selection")
	}
	if e.isDir() {
		return errors.New("copy directory: not supported")
	}
	if e.Name == ".." {
		return errors.New("copy: invalid selection")
	}

	dst := joinPath(dstPanel.path, e.Name)
	data, err := t.readAll(ctx, e.FullPath, maxCopyBytes)
	if err != nil {
		return err
	}
	if _, err := t.vfsClient().Write(ctx, dst, proto.VFSWriteTruncate, data); err != nil {
		return err
	}
	_ = t.loadDir(ctx, dstPanel)
	t.setMessage("copied to " + dst)
	return nil
}

func (t *Task) openViewer(ctx *kernel.Context, p string) error {
	data, err := t.readAll(ctx, p, maxViewerBytes)
	if err != nil {
		return err
	}
	lines := decodeLines(data)
	if len(lines) == 0 {
		lines = [][]rune{{}}
	}
	t.viewerPath = p
	t.viewerLines = lines
	t.viewerTop = 0
	t.mode = modeViewer
	return nil
}

func (t *Task) handleViewerKey(ctx *kernel.Context, k key) {
	switch k.kind {
	case keyEsc:
		t.mode = modePanels
		return
	case keyUp:
		if t.viewerTop > 0 {
			t.viewerTop--
		}
	case keyDown:
		if t.viewerTop+1 < len(t.viewerLines) {
			t.viewerTop++
		}
	case keyHome:
		t.viewerTop = 0
	case keyEnd:
		t.viewerTop = len(t.viewerLines) - 1
	case keyRune:
		switch k.r {
		case 'q':
			t.mode = modePanels
		case 'x':
			if err := t.openHex(ctx, t.viewerPath); err != nil {
				t.setMessage("hex: " + err.Error())
			}
		case 'j':
			if t.viewerTop+1 < len(t.viewerLines) {
				t.viewerTop++
			}
		case 'k':
			if t.viewerTop > 0 {
				t.viewerTop--
			}
		}
	}
}

func (t *Task) openHex(ctx *kernel.Context, p string) error {
	data, err := t.readAll(ctx, p, maxHexBytes)
	if err != nil {
		return err
	}
	t.hexPath = p
	t.hexData = data
	t.hexCursor = 0
	t.hexTop = 0
	t.hexNibble = 0
	t.hexDirty = false
	t.hexQuitConfirm = false
	t.hexEditASCII = false
	t.mode = modeHex
	return nil
}

func (t *Task) handleHexKey(ctx *kernel.Context, k key) {
	bytesPerRow, _ := t.hexLayout()
	if bytesPerRow <= 0 {
		bytesPerRow = 16
	}

	if k.kind != keyRune || (k.r != 'w' && k.r != 'q') {
		t.hexQuitConfirm = false
	}

	switch k.kind {
	case keyEsc:
		if t.hexDirty && !t.hexQuitConfirm {
			t.hexQuitConfirm = true
			t.setMessage("unsaved: w save | q discard")
			return
		}
		t.mode = modePanels
		return

	case keyLeft:
		if t.hexCursor > 0 {
			t.hexCursor--
		}
		t.hexNibble = 0
	case keyRight:
		if t.hexCursor+1 < len(t.hexData) {
			t.hexCursor++
		}
		t.hexNibble = 0
	case keyUp:
		if t.hexCursor-bytesPerRow >= 0 {
			t.hexCursor -= bytesPerRow
		}
		t.hexNibble = 0
	case keyDown:
		if t.hexCursor+bytesPerRow < len(t.hexData) {
			t.hexCursor += bytesPerRow
		}
		t.hexNibble = 0
	case keyHome:
		t.hexCursor = 0
		t.hexNibble = 0
	case keyEnd:
		if len(t.hexData) > 0 {
			t.hexCursor = len(t.hexData) - 1
		} else {
			t.hexCursor = 0
		}
		t.hexNibble = 0

	case keyRune:
		switch k.r {
		case 'q':
			if t.hexDirty && !t.hexQuitConfirm {
				t.hexQuitConfirm = true
				t.setMessage("unsaved: w save | q discard")
				return
			}
			t.mode = modePanels
			return
		case 't':
			if err := t.openViewer(ctx, t.hexPath); err != nil {
				t.setMessage("view: " + err.Error())
			}
			return
		case 'i':
			t.hexEditASCII = !t.hexEditASCII
			if t.hexEditASCII {
				t.setMessage("edit: ASCII")
			} else {
				t.setMessage("edit: HEX")
			}
			return
		case 'w':
			if !t.hexDirty {
				t.setMessage("saved")
				return
			}
			if _, err := t.vfsClient().Write(ctx, t.hexPath, proto.VFSWriteTruncate, t.hexData); err != nil {
				t.setMessage("save: " + err.Error())
				return
			}
			t.hexDirty = false
			t.hexQuitConfirm = false
			_ = t.refreshPanels(ctx)
			t.setMessage("saved")
			return
		default:
			t.handleHexEditRune(k.r)
		}
	}

	t.hexEnsureVisible(bytesPerRow)
}

func (t *Task) handleHexEditRune(r rune) {
	if len(t.hexData) == 0 || t.hexCursor < 0 || t.hexCursor >= len(t.hexData) {
		return
	}

	if t.hexEditASCII {
		if r < 0x20 || r > 0x7e {
			return
		}
		prev := t.hexData[t.hexCursor]
		next := byte(r)
		if prev != next {
			t.hexData[t.hexCursor] = next
			t.hexDirty = true
		}
		if t.hexCursor+1 < len(t.hexData) {
			t.hexCursor++
		}
		t.hexNibble = 0
		return
	}

	val, ok := hexDigitValue(r)
	if !ok {
		return
	}

	idx := t.hexCursor
	prev := t.hexData[idx]
	next := prev
	if t.hexNibble == 0 {
		next = (prev & 0x0f) | (val << 4)
		t.hexNibble = 1
	} else {
		next = (prev & 0xf0) | val
		t.hexNibble = 0
	}
	if next != prev {
		t.hexData[idx] = next
		t.hexDirty = true
	}

	if t.hexNibble == 0 && t.hexCursor+1 < len(t.hexData) {
		t.hexCursor++
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

func (t *Task) hexLayout() (bytesPerRow int, showASCII bool) {
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

func (t *Task) hexEnsureVisible(bytesPerRow int) {
	if bytesPerRow <= 0 || t.viewRows <= 0 || len(t.hexData) == 0 {
		t.hexTop = 0
		return
	}

	row := t.hexCursor / bytesPerRow
	if row < 0 {
		row = 0
	}
	if row < t.hexTop {
		t.hexTop = row
	} else if row >= t.hexTop+t.viewRows {
		t.hexTop = row - (t.viewRows - 1)
	}

	maxTop := (len(t.hexData)-1)/bytesPerRow - (t.viewRows - 1)
	if maxTop < 0 {
		maxTop = 0
	}
	if t.hexTop < 0 {
		t.hexTop = 0
	}
	if t.hexTop > maxTop {
		t.hexTop = maxTop
	}
}

func (t *Task) readAll(ctx *kernel.Context, path string, maxBytes int) ([]byte, error) {
	var out []byte
	var off uint32
	for {
		chunk, eof, err := t.vfsClient().ReadAt(ctx, path, off, maxVFSRead)
		if err != nil {
			return nil, err
		}
		if len(chunk) > 0 {
			out = append(out, chunk...)
			if maxBytes > 0 && len(out) > maxBytes {
				return nil, fmt.Errorf("file too large (>%d bytes)", maxBytes)
			}
			off += uint32(len(chunk))
		}
		if eof {
			return out, nil
		}
	}
}

func decodeLines(data []byte) [][]rune {
	s := string(data)
	parts := strings.Split(s, "\n")
	if len(parts) > 0 && parts[len(parts)-1] == "" && strings.HasSuffix(s, "\n") {
		parts = parts[:len(parts)-1]
	}
	if len(parts) == 0 {
		return [][]rune{{}}
	}

	lines := make([][]rune, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSuffix(p, "\r")
		lines = append(lines, []rune(p))
	}
	if len(lines) == 0 {
		lines = [][]rune{{}}
	}
	return lines
}

func (t *Task) headerText() string {
	left := t.left.path
	right := t.right.path
	if t.activePanel == panelLeft {
		left = "[" + left + "]"
	} else {
		right = "[" + right + "]"
	}
	leftClip := clipRunes(left, t.panelCols)
	rightClip := clipRunes(right, t.panelCols)
	leftPad := t.panelCols - len([]rune(leftClip))
	if leftPad < 0 {
		leftPad = 0
	}
	return leftClip + padSpaces(leftPad) + "|" + rightClip
}

func (t *Task) statusText() string {
	if t.message == "" {
		return ""
	}
	return clipRunes(t.message, t.cols)
}

func (t *Task) viewerHeaderText() string {
	return clipRunes("VIEW "+t.viewerPath, t.cols)
}

func (t *Task) viewerStatusText() string {
	msg := "q/ESC back"
	if len(t.viewerLines) > 0 {
		msg = fmt.Sprintf("%s | %d/%d", msg, t.viewerTop+1, len(t.viewerLines))
	}
	return clipRunes(msg, t.cols)
}

func (t *Task) hexHeaderText() string {
	return clipRunes("HEX "+t.hexPath, t.cols)
}

func (t *Task) hexStatusText() string {
	var flags string
	if t.hexDirty {
		flags = "*"
	}
	edit := "HEX"
	if t.hexEditASCII {
		edit = "ASCII"
	}

	nibble := ""
	if !t.hexEditASCII {
		if t.hexNibble == 0 {
			nibble = " hi"
		} else {
			nibble = " lo"
		}
	}

	base := fmt.Sprintf(
		"%soff=%08X size=%08X %s%s | i edit | w save | t text | q back",
		flags,
		t.hexCursor,
		len(t.hexData),
		edit,
		nibble,
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

func padSpaces(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat(" ", n)
}
