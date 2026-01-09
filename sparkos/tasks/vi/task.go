//go:build spark_vi

package vi

import (
	"errors"
	"fmt"
	"path"
	"strings"
	"unicode/utf8"

	"spark/hal"
	vfsclient "spark/sparkos/client/vfs"
	"spark/sparkos/fonts/font6x8cp1251"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"

	"tinygo.org/x/tinyfont"
)

// Enabled reports whether SparkVi is compiled into this build.
const Enabled = true

const (
	maxFileBytes = 512 * 1024

	maxVFSRead = kernel.MaxMessageBytes - 11
)

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

	editor editor

	inbuf []byte
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

	t.font = font6x8cp1251.Font
	t.fontHeight = 8
	t.fontOffset = 7
	_, outboxWidth := tinyfont.LineWidth(t.font, "0")
	t.fontWidth = int16(outboxWidth)
	if t.fontWidth <= 0 || t.fontHeight <= 0 {
		return
	}

	t.cols = t.fb.Width() / int(t.fontWidth)
	t.rows = t.fb.Height() / int(t.fontHeight)
	t.viewRows = t.rows - 1
	if t.cols <= 0 || t.viewRows <= 0 {
		return
	}

	t.editor.reset()
	t.editor.ensureCursorVisible(t.viewRows, t.cols)

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
			t.setActive(active)
			if t.active {
				t.render()
			}

		case proto.MsgAppSelect:
			appID, arg, ok := proto.DecodeAppSelectPayload(msg.Payload())
			if !ok || appID != proto.AppVi {
				continue
			}
			if arg != "" {
				if err := t.openPath(ctx, cleanPath(arg)); err != nil {
					t.editor.setMessage(err.Error())
				}
			}
			if t.active {
				t.render()
			}

		case proto.MsgTermInput:
			if !t.active {
				continue
			}
			exit := t.handleInput(ctx, msg.Payload())
			if exit {
				t.requestExit(ctx)
				continue
			}
			t.render()
		}
	}
}

func (t *Task) setActive(active bool) {
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

	if len(t.editor.lines) == 0 {
		t.editor.reset()
		t.editor.ensureCursorVisible(t.viewRows, t.cols)
	}
	if t.editor.filePath == "" {
		t.editor.setMessage("SparkVi: :q to exit, :w to save, :e <file> to open.")
	}
}

func (t *Task) requestExit(ctx *kernel.Context) {
	if !t.muxCap.Valid() {
		t.active = false
		t.unloadSession()
		return
	}
	_ = ctx.SendToCapRetry(t.muxCap, uint16(proto.MsgAppControl), proto.AppControlPayload(false), kernel.Capability{}, 500)
	t.active = false
	t.unloadSession()
}

func (t *Task) unloadSession() {
	t.inbuf = nil
	t.vfs = nil
	t.editor = editor{}
}

func (t *Task) vfsClient() *vfsclient.Client {
	if t.vfs == nil {
		t.vfs = vfsclient.New(t.vfsCap)
	}
	return t.vfs
}

func (t *Task) openPath(ctx *kernel.Context, p string) error {
	if p == "" || p == "/" || strings.HasSuffix(p, "/") {
		return errors.New("vi: invalid path")
	}

	data, err := t.readAll(ctx, p)
	if err != nil {
		if isNotFound(err) {
			t.editor.reset()
			t.editor.filePath = p
			t.editor.modified = false
			t.editor.setMessage(fmt.Sprintf("\"%s\" [New File]", p))
			return nil
		}
		return fmt.Errorf("vi: open %s: %w", p, err)
	}

	lines := decodeLines(data)
	t.editor.reset()
	t.editor.filePath = p
	t.editor.lines = lines
	t.editor.modified = false
	t.editor.ensureCursorVisible(t.viewRows, t.cols)
	t.editor.setMessage(fmt.Sprintf("\"%s\" %d lines", p, len(lines)))
	return nil
}

func (t *Task) readAll(ctx *kernel.Context, p string) ([]byte, error) {
	var out []byte
	var off uint32
	for {
		chunk, eof, err := t.vfsClient().ReadAt(ctx, p, off, maxVFSRead)
		if err != nil {
			return nil, err
		}
		if len(chunk) > 0 {
			out = append(out, chunk...)
			if len(out) > maxFileBytes {
				return nil, errors.New("file too large")
			}
			off += uint32(len(chunk))
		}
		if eof {
			return out, nil
		}
	}
}

func (t *Task) save(ctx *kernel.Context, p string) error {
	p = cleanPath(p)
	if p == "" || p == "/" || strings.HasSuffix(p, "/") {
		return errors.New("vi: invalid path")
	}

	data := encodeLines(t.editor.lines)
	if len(data) > maxFileBytes {
		return errors.New("file too large")
	}

	n, err := t.vfsClient().Write(ctx, p, proto.VFSWriteTruncate, data)
	if err != nil {
		return fmt.Errorf("vi: write %s: %w", p, err)
	}

	t.editor.filePath = p
	t.editor.modified = false
	t.editor.setMessage(fmt.Sprintf("\"%s\" %d bytes written", p, n))
	return nil
}

func (t *Task) handleInput(ctx *kernel.Context, b []byte) (exit bool) {
	t.inbuf = append(t.inbuf, b...)
	buf := t.inbuf

	for len(buf) > 0 {
		n, k, ok := nextKey(buf)
		if !ok {
			break
		}
		buf = buf[n:]

		exit = t.handleKey(ctx, k)
		if exit {
			t.inbuf = t.inbuf[:0]
			return true
		}
	}

	t.inbuf = append(t.inbuf[:0], buf...)
	return false
}

func (t *Task) handleKey(ctx *kernel.Context, k key) (exit bool) {
	switch t.editor.mode {
	case modeCmdline:
		switch k.kind {
		case keyEnter:
			cmd := t.editor.takeCmdline()
			t.editor.mode = modeNormal
			return t.execExCommand(ctx, cmd)
		case keyEsc:
			t.editor.clearCmdline()
			t.editor.mode = modeNormal
		default:
			t.editor.handleCmdlineKey(k)
		}
		return false

	case modeSearch:
		switch k.kind {
		case keyEnter:
			pat := t.editor.takeSearch()
			t.editor.mode = modeNormal
			t.editor.searchForward(pat)
		case keyEsc:
			t.editor.clearSearch()
			t.editor.mode = modeNormal
		default:
			t.editor.handleSearchKey(k)
		}
		return false
	}

	switch t.editor.mode {
	case modeInsert:
		exit = t.editor.handleInsertKey(k)
		if exit {
			return true
		}
	case modeNormal:
		exit = t.editor.handleNormalKey(k)
		if exit {
			return true
		}
	}

	t.editor.ensureCursorVisible(t.viewRows, t.cols)
	return false
}

func (t *Task) execExCommand(ctx *kernel.Context, cmd string) (exit bool) {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return false
	}

	force := strings.HasSuffix(cmd, "!")
	if force {
		cmd = strings.TrimSuffix(cmd, "!")
		cmd = strings.TrimSpace(cmd)
	}

	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return false
	}

	switch parts[0] {
	case "q", "quit":
		if t.editor.modified && !force {
			t.editor.setMessage("No write since last change (add ! to override).")
			return false
		}
		return true

	case "w", "write":
		target := t.editor.filePath
		if len(parts) >= 2 {
			target = resolvePath(t.editor.filePath, parts[1])
		}
		if target == "" {
			t.editor.setMessage("No file name.")
			return false
		}
		if err := t.save(ctx, target); err != nil {
			t.editor.setMessage(err.Error())
		}
		return false

	case "wq":
		target := t.editor.filePath
		if len(parts) >= 2 {
			target = resolvePath(t.editor.filePath, parts[1])
		}
		if target == "" {
			t.editor.setMessage("No file name.")
			return false
		}
		if err := t.save(ctx, target); err != nil {
			t.editor.setMessage(err.Error())
			return false
		}
		return true

	case "e", "edit":
		if len(parts) != 2 {
			t.editor.setMessage("usage: :e <file>")
			return false
		}
		if t.editor.modified && !force {
			t.editor.setMessage("No write since last change (add ! to override).")
			return false
		}
		target := resolvePath(t.editor.filePath, parts[1])
		if err := t.openPath(ctx, target); err != nil {
			t.editor.setMessage(err.Error())
		}
		return false

	default:
		t.editor.setMessage("Not an editor command: " + parts[0])
		return false
	}
}

func resolvePath(baseFile, p string) string {
	if p == "" {
		return ""
	}
	if strings.HasPrefix(p, "/") {
		return cleanPath(p)
	}
	baseDir := "/"
	if baseFile != "" {
		baseDir = path.Dir(baseFile)
	}
	if baseDir == "/" {
		return cleanPath("/" + p)
	}
	return cleanPath(baseDir + "/" + p)
}

func cleanPath(p string) string {
	p = path.Clean(p)
	if p == "." {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), proto.ErrNotFound.String())
}

func nextKey(b []byte) (consumed int, k key, ok bool) {
	if len(b) == 0 {
		return 0, key{}, false
	}

	if b[0] == 0x1b {
		return parseEscapeKey(b)
	}

	switch b[0] {
	case '\r':
		return 1, key{kind: keyEnter}, true
	case '\n':
		return 1, key{kind: keyEnter}, true
	case 0x7f, 0x08:
		return 1, key{kind: keyBackspace}, true
	case '\t':
		return 1, key{kind: keyTab}, true
	}

	if b[0] < 0x20 {
		return 1, key{kind: keyCtrl, ctrl: b[0]}, true
	}
	if !utf8.FullRune(b) {
		return 0, key{}, false
	}
	r, sz := utf8.DecodeRune(b)
	if r == utf8.RuneError && sz == 1 {
		return 1, key{}, true
	}
	return sz, key{kind: keyRune, r: r}, true
}

func parseEscapeKey(b []byte) (consumed int, k key, ok bool) {
	if len(b) < 2 {
		return 1, key{kind: keyEsc}, true
	}
	if b[1] != '[' {
		return 1, key{kind: keyEsc}, true
	}
	if len(b) < 3 {
		return 0, key{}, false
	}

	switch b[2] {
	case 'A':
		return 3, key{kind: keyUp}, true
	case 'B':
		return 3, key{kind: keyDown}, true
	case 'C':
		return 3, key{kind: keyRight}, true
	case 'D':
		return 3, key{kind: keyLeft}, true
	case 'H':
		return 3, key{kind: keyHome}, true
	case 'F':
		return 3, key{kind: keyEnd}, true
	case '3':
		if len(b) < 4 {
			return 0, key{}, false
		}
		if b[3] == '~' {
			return 4, key{kind: keyDelete}, true
		}
		return 1, key{kind: keyEsc}, true
	case '1':
		if len(b) < 4 {
			return 0, key{}, false
		}
		if b[3] == '~' {
			return 4, key{kind: keyHome}, true
		}
		return 1, key{kind: keyEsc}, true
	case '4':
		if len(b) < 4 {
			return 0, key{}, false
		}
		if b[3] == '~' {
			return 4, key{kind: keyEnd}, true
		}
		return 1, key{kind: keyEsc}, true
	default:
		return 1, key{kind: keyEsc}, true
	}
}
