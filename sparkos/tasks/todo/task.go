package todo

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"spark/hal"
	vfsclient "spark/sparkos/client/vfs"
	"spark/sparkos/fonts/font6x8cp1251"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"

	"tinygo.org/x/tinyfont"
)

type filterMode uint8

const (
	filterAll filterMode = iota
	filterOpen
	filterDone
)

type inputMode uint8

const (
	inputNone inputMode = iota
	inputAdd
	inputEdit
	inputSearch
	inputConfirmDelete
)

type item struct {
	id   uint32
	done bool
	prio uint8 // 0..2
	text string
}

type Task struct {
	disp   hal.Display
	ep     kernel.Capability
	vfsCap kernel.Capability

	fb hal.Framebuffer

	font       tinyfont.Fonter
	fontWidth  int16
	fontHeight int16

	active bool
	muxCap kernel.Capability

	w int
	h int

	nowTick uint64

	vfs *vfsclient.Client

	items []item

	filter filterMode
	search string

	visible []int

	sel int
	top int

	nextID uint32
	dirty  bool

	status string

	inbuf []byte

	inputMode   inputMode
	inputPrompt string
	input       []rune
	inputCursor int
}

const (
	dirPath   = "/todo"
	itemsPath = "/todo/items.txt"
	statePath = "/todo/state.txt"
)

func New(disp hal.Display, ep kernel.Capability, vfsCap kernel.Capability) *Task {
	return &Task{disp: disp, ep: ep, vfsCap: vfsCap, nextID: 1}
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
	if !t.initFont() {
		return
	}

	done := make(chan struct{})
	defer close(done)

	tickCh := make(chan uint64, 8)
	go func() {
		last := ctx.NowTick()
		for {
			select {
			case <-done:
				return
			default:
			}
			last = ctx.WaitTick(last)
			select {
			case tickCh <- last:
			default:
			}
		}
	}()

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}
			switch proto.Kind(msg.Kind) {
			case proto.MsgAppShutdown:
				t.unload()
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
				if !ok || appID != proto.AppTodo {
					continue
				}
				t.applyArg(arg)
				if t.active {
					t.render()
				}

			case proto.MsgTermInput:
				if !t.active {
					continue
				}
				t.handleInput(ctx, msg.Payload())
				if t.active {
					t.render()
				}
			}

		case now := <-tickCh:
			if !t.active {
				continue
			}
			t.nowTick = now
			if t.inputMode != inputNone && (now/350)%2 == 0 {
				t.render()
			}
		}
	}
}

func (t *Task) initFont() bool {
	t.font = font6x8cp1251.Font
	t.fontHeight = 8
	_, outboxWidth := tinyfont.LineWidth(t.font, "0")
	t.fontWidth = int16(outboxWidth)
	return t.fontWidth > 0 && t.fontHeight > 0
}

func (t *Task) setActive(ctx *kernel.Context, active bool) {
	if active == t.active {
		return
	}
	t.active = active
	if !t.active {
		return
	}
	t.initApp(ctx)
	t.render()
}

func (t *Task) initApp(ctx *kernel.Context) {
	t.w = t.fb.Width()
	t.h = t.fb.Height()
	if t.w <= 0 || t.h <= 0 {
		t.active = false
		return
	}

	if t.vfs == nil && t.vfsCap.Valid() {
		t.vfs = vfsclient.New(t.vfsCap)
	}
	t.status = ""
	t.inputMode = inputNone
	t.inputPrompt = ""
	t.input = t.input[:0]
	t.inputCursor = 0
	t.filter = filterAll
	t.search = ""
	t.dirty = false

	t.loadState(ctx)
	t.loadItems(ctx)
	t.rebuildVisible()
	t.ensureSelectionInRange()
}

func (t *Task) unload() {
	t.active = false
	t.items = nil
	t.visible = nil
	t.inbuf = nil
	t.input = nil
	t.vfs = nil
}

func (t *Task) requestExit(ctx *kernel.Context) {
	t.saveState(ctx)
	if t.dirty {
		t.saveItems(ctx)
	}

	t.active = false
	if !t.muxCap.Valid() {
		return
	}
	_ = ctx.SendToCapRetry(t.muxCap, uint16(proto.MsgAppControl), proto.AppControlPayload(false), kernel.Capability{}, 500)
}

func (t *Task) applyArg(arg string) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return
	}
	switch arg {
	case "all":
		t.filter = filterAll
	case "open":
		t.filter = filterOpen
	case "done":
		t.filter = filterDone
	default:
		t.search = arg
	}
	t.rebuildVisible()
	t.ensureSelectionInRange()
}

func (t *Task) handleInput(ctx *kernel.Context, b []byte) {
	t.nowTick = ctx.NowTick()
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
	if t.inputMode != inputNone {
		t.handleInputKey(ctx, k)
		return
	}

	switch k.kind {
	case keyEsc:
		t.requestExit(ctx)
	case keyUp:
		t.moveSel(-1)
	case keyDown:
		t.moveSel(1)
	case keyEnter:
		t.toggleDone()
	case keyRune:
		t.handleRune(ctx, k.r)
	}
}

func (t *Task) handleRune(ctx *kernel.Context, r rune) {
	switch r {
	case 'q':
		t.requestExit(ctx)
	case 'a':
		t.beginAdd()
	case 'e':
		t.beginEdit()
	case 'd':
		t.beginDelete()
	case ' ':
		t.toggleDone()
	case 'p':
		t.cyclePriority()
	case 'f':
		t.cycleFilter()
	case '/':
		t.beginSearch()
	case 's':
		t.saveItems(ctx)
		t.status = "Saved."
	case 'j':
		t.moveSel(1)
	case 'k':
		t.moveSel(-1)
	case 'J':
		t.moveItem(1)
	case 'K':
		t.moveItem(-1)
	}
}

func (t *Task) handleInputKey(ctx *kernel.Context, k key) {
	switch k.kind {
	case keyEsc:
		t.inputMode = inputNone
		t.inputPrompt = ""
		t.input = t.input[:0]
		t.inputCursor = 0
		t.status = "Canceled."
	case keyEnter:
		text := strings.TrimSpace(string(t.input))
		mode := t.inputMode
		t.inputMode = inputNone
		t.inputPrompt = ""
		t.input = t.input[:0]
		t.inputCursor = 0
		t.applyInput(ctx, mode, text)
	case keyBackspace:
		if t.inputCursor <= 0 || len(t.input) == 0 {
			return
		}
		copy(t.input[t.inputCursor-1:], t.input[t.inputCursor:])
		t.input = t.input[:len(t.input)-1]
		t.inputCursor--
	case keyLeft:
		if t.inputCursor > 0 {
			t.inputCursor--
		}
	case keyRight:
		if t.inputCursor < len(t.input) {
			t.inputCursor++
		}
	case keyRune:
		if len(t.input) >= 160 {
			return
		}
		t.input = append(t.input, 0)
		copy(t.input[t.inputCursor+1:], t.input[t.inputCursor:])
		t.input[t.inputCursor] = k.r
		t.inputCursor++
	}
}

func (t *Task) applyInput(ctx *kernel.Context, mode inputMode, text string) {
	switch mode {
	case inputAdd:
		if text == "" {
			t.status = "Empty item."
			return
		}
		t.items = append(t.items, item{id: t.allocID(), prio: 1, text: text})
		t.dirty = true
		t.status = "Added."
		t.rebuildVisible()
		t.sel = len(t.visible) - 1
		t.ensureSelectionInRange()
		t.ensureSelectionVisible()

	case inputEdit:
		idx, ok := t.selectedIndex()
		if !ok {
			t.status = "No selection."
			return
		}
		if text == "" {
			t.status = "Empty item."
			return
		}
		t.items[idx].text = text
		t.dirty = true
		t.status = "Updated."
		t.rebuildVisible()
		t.ensureSelectionInRange()
		t.ensureSelectionVisible()

	case inputSearch:
		t.search = text
		t.rebuildVisible()
		t.ensureSelectionInRange()
		t.status = "Search applied."

	case inputConfirmDelete:
		if strings.ToLower(strings.TrimSpace(text)) != "y" {
			t.status = "Not deleted."
			return
		}
		t.deleteSelected()
		t.status = "Deleted."
		t.saveItems(ctx)
	}
}

func (t *Task) allocID() uint32 {
	if t.nextID == 0 {
		t.nextID = 1
	}
	id := t.nextID
	t.nextID++
	if t.nextID == 0 {
		t.nextID = 1
	}
	return id
}

func (t *Task) selectedIndex() (int, bool) {
	if t.sel < 0 || t.sel >= len(t.visible) {
		return 0, false
	}
	idx := t.visible[t.sel]
	if idx < 0 || idx >= len(t.items) {
		return 0, false
	}
	return idx, true
}

func (t *Task) ensureSelectionInRange() {
	if len(t.visible) == 0 {
		t.sel = 0
		t.top = 0
		return
	}
	if t.sel < 0 {
		t.sel = 0
	}
	if t.sel >= len(t.visible) {
		t.sel = len(t.visible) - 1
	}
	if t.top < 0 {
		t.top = 0
	}
	if t.top > t.sel {
		t.top = t.sel
	}
}

func (t *Task) moveSel(delta int) {
	if len(t.visible) == 0 {
		return
	}
	t.sel += delta
	if t.sel < 0 {
		t.sel = 0
	}
	if t.sel >= len(t.visible) {
		t.sel = len(t.visible) - 1
	}
	t.ensureSelectionVisible()
}

func (t *Task) ensureSelectionVisible() {
	listY, listH, _ := t.layout()
	if listH <= 0 {
		return
	}
	lineH := int(t.fontHeight) + 2
	rows := listH / lineH
	if rows < 1 {
		rows = 1
	}
	_ = listY
	if t.sel < t.top {
		t.top = t.sel
	}
	if t.sel >= t.top+rows {
		t.top = t.sel - rows + 1
	}
	if t.top < 0 {
		t.top = 0
	}
	if t.top > t.sel {
		t.top = t.sel
	}
}

func (t *Task) toggleDone() {
	idx, ok := t.selectedIndex()
	if !ok {
		return
	}
	t.items[idx].done = !t.items[idx].done
	t.dirty = true
	t.rebuildVisible()
	t.ensureSelectionInRange()
	t.ensureSelectionVisible()
}

func (t *Task) cyclePriority() {
	idx, ok := t.selectedIndex()
	if !ok {
		return
	}
	t.items[idx].prio = (t.items[idx].prio + 1) % 3
	t.dirty = true
}

func (t *Task) cycleFilter() {
	t.filter = (t.filter + 1) % 3
	t.rebuildVisible()
	t.ensureSelectionInRange()
	t.ensureSelectionVisible()
}

func (t *Task) beginAdd() {
	t.inputMode = inputAdd
	t.inputPrompt = "Add todo: "
	t.input = t.input[:0]
	t.inputCursor = 0
}

func (t *Task) beginEdit() {
	idx, ok := t.selectedIndex()
	if !ok {
		t.status = "No selection."
		return
	}
	t.inputMode = inputEdit
	t.inputPrompt = "Edit todo: "
	t.input = []rune(t.items[idx].text)
	t.inputCursor = len(t.input)
}

func (t *Task) beginSearch() {
	t.inputMode = inputSearch
	t.inputPrompt = "Search: "
	t.input = []rune(t.search)
	t.inputCursor = len(t.input)
}

func (t *Task) beginDelete() {
	if _, ok := t.selectedIndex(); !ok {
		t.status = "No selection."
		return
	}
	t.inputMode = inputConfirmDelete
	t.inputPrompt = "Delete? type y + Enter: "
	t.input = t.input[:0]
	t.inputCursor = 0
}

func (t *Task) deleteSelected() {
	idx, ok := t.selectedIndex()
	if !ok {
		return
	}
	t.items = append(t.items[:idx], t.items[idx+1:]...)
	t.dirty = true
	t.rebuildVisible()
	t.ensureSelectionInRange()
	t.ensureSelectionVisible()
}

func (t *Task) moveItem(delta int) {
	if len(t.visible) == 0 {
		return
	}
	target := t.sel + delta
	if target < 0 || target >= len(t.visible) {
		return
	}
	idxA := t.visible[t.sel]
	idxB := t.visible[target]
	if idxA < 0 || idxA >= len(t.items) || idxB < 0 || idxB >= len(t.items) {
		return
	}
	t.items[idxA], t.items[idxB] = t.items[idxB], t.items[idxA]
	t.dirty = true
	t.rebuildVisible()
	t.sel = target
	t.ensureSelectionInRange()
	t.ensureSelectionVisible()
}

func (t *Task) rebuildVisible() {
	t.visible = t.visible[:0]
	for i := 0; i < len(t.items); i++ {
		it := t.items[i]
		if t.filter == filterOpen && it.done {
			continue
		}
		if t.filter == filterDone && !it.done {
			continue
		}
		if t.search != "" && !strings.Contains(strings.ToLower(it.text), strings.ToLower(t.search)) {
			continue
		}
		t.visible = append(t.visible, i)
	}
}

func (t *Task) ensureTodoDir(ctx *kernel.Context) error {
	if t.vfs == nil {
		return fmt.Errorf("todo: vfs not available")
	}
	typ, _, err := t.vfs.Stat(ctx, dirPath)
	if err == nil {
		if typ == proto.VFSEntryDir {
			return nil
		}
		return fmt.Errorf("todo: %s is not a directory", dirPath)
	}
	if err := t.vfs.Mkdir(ctx, dirPath); err == nil {
		return nil
	}
	typ, _, err = t.vfs.Stat(ctx, dirPath)
	if err != nil {
		return fmt.Errorf("todo: ensure %s: %w", dirPath, err)
	}
	if typ != proto.VFSEntryDir {
		return fmt.Errorf("todo: %s is not a directory", dirPath)
	}
	return nil
}

func (t *Task) loadState(ctx *kernel.Context) {
	if t.vfs == nil {
		return
	}
	if err := t.ensureTodoDir(ctx); err != nil {
		return
	}
	typ, size, err := t.vfs.Stat(ctx, statePath)
	if err != nil || typ != proto.VFSEntryFile || size == 0 || size > 64 {
		return
	}
	b, _, err := readAll(ctx, t.vfs, statePath, size, 128)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, val, ok := splitKV(line)
		if !ok {
			continue
		}
		switch key {
		case "next":
			if v, err := strconv.ParseUint(val, 10, 32); err == nil && v > 0 {
				t.nextID = uint32(v)
			}
		}
	}
}

func (t *Task) saveState(ctx *kernel.Context) {
	if t.vfs == nil {
		return
	}
	if err := t.ensureTodoDir(ctx); err != nil {
		return
	}
	s := fmt.Sprintf("next=%d\n", t.nextID)
	_, _ = t.vfs.Write(ctx, statePath, proto.VFSWriteTruncate, []byte(s))
}

func (t *Task) loadItems(ctx *kernel.Context) {
	if t.vfs == nil {
		return
	}
	if err := t.ensureTodoDir(ctx); err != nil {
		return
	}
	typ, size, err := t.vfs.Stat(ctx, itemsPath)
	if err != nil || typ != proto.VFSEntryFile || size == 0 {
		return
	}
	if size > 128*1024 {
		t.status = "todo file too large."
		return
	}
	b, _, err := readAll(ctx, t.vfs, itemsPath, size, uint16(size))
	if err != nil {
		t.status = "failed to read todo file."
		return
	}

	var items []item
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := splitFields(line, '|')
		if len(fields) < 4 {
			continue
		}
		id, err := strconv.ParseUint(fields[0], 10, 32)
		if err != nil || id == 0 {
			continue
		}
		done := fields[1] == "1"
		prio, _ := strconv.ParseUint(fields[2], 10, 8)
		text, ok := unescapeField(fields[3])
		if !ok || strings.TrimSpace(text) == "" {
			continue
		}
		items = append(items, item{id: uint32(id), done: done, prio: uint8(prio % 3), text: text})
		if uint32(id) >= t.nextID {
			t.nextID = uint32(id) + 1
		}
	}
	t.items = items
}

func (t *Task) saveItems(ctx *kernel.Context) {
	if t.vfs == nil {
		return
	}
	if err := t.ensureTodoDir(ctx); err != nil {
		return
	}

	var b strings.Builder
	b.WriteString("# SparkOS TODO items.\n")
	for i := 0; i < len(t.items); i++ {
		it := t.items[i]
		done := "0"
		if it.done {
			done = "1"
		}
		fmt.Fprintf(&b, "%d|%s|%d|%s\n", it.id, done, it.prio%3, escapeField(it.text))
	}
	_, _ = t.vfs.Write(ctx, itemsPath, proto.VFSWriteTruncate, []byte(b.String()))
	t.dirty = false
}

func readAll(ctx *kernel.Context, c *vfsclient.Client, path string, size uint32, maxChunk uint16) ([]byte, bool, error) {
	if size == 0 {
		return nil, true, nil
	}
	out := make([]byte, 0, int(size))
	off := uint32(0)
	for {
		chunk, eof, err := c.ReadAt(ctx, path, off, maxChunk)
		if err != nil {
			return nil, false, err
		}
		out = append(out, chunk...)
		off += uint32(len(chunk))
		if eof || len(chunk) == 0 {
			return out, eof, nil
		}
		if off >= size {
			return out, true, nil
		}
	}
}

func splitKV(s string) (key, val string, ok bool) {
	i := strings.IndexByte(s, '=')
	if i < 0 {
		return "", "", false
	}
	return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+1:]), true
}

func splitFields(s string, sep byte) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			out = append(out, strings.TrimSpace(s[start:i]))
			start = i + 1
		}
	}
	out = append(out, strings.TrimSpace(s[start:]))
	return out
}

func escapeField(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}

func unescapeField(s string) (string, bool) {
	var b strings.Builder
	escaped := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if !escaped {
			if ch == '\\' {
				escaped = true
				continue
			}
			b.WriteByte(ch)
			continue
		}
		escaped = false
		switch ch {
		case '\\':
			b.WriteByte('\\')
		case '|':
			b.WriteByte('|')
		case 'n':
			b.WriteByte('\n')
		default:
			return "", false
		}
	}
	if escaped {
		return "", false
	}
	return b.String(), true
}

func (t *Task) layout() (listY, listH int, panelW int) {
	top := int(t.fontHeight)*2 + 10
	bottomReserved := int(t.fontHeight) + 6
	if t.inputMode == inputNone {
		bottomReserved = int(t.fontHeight) + 4
	}

	listY = top
	listH = t.h - top - bottomReserved - 6
	if listH < 0 {
		listH = 0
	}

	panelW = 0
	if t.w >= 320 {
		panelW = 140
	}
	return listY, listH, panelW
}

func (t *Task) render() {
	if !t.active || t.fb == nil || t.fb.Format() != hal.PixelFormatRGB565 {
		return
	}
	buf := t.fb.Buffer()
	if buf == nil {
		return
	}

	bg := rgb565From888(0x08, 0x0B, 0x10)
	clearRGB565(buf, bg)

	title := "TODO"
	stats := t.statsLine()
	t.drawText(6, 0, title, color.RGBA{R: 0xEE, G: 0xEE, B: 0xEE, A: 0xFF})
	t.drawText(6, int(t.fontHeight)+1, truncateToWidth(t.font, stats, t.w-12), color.RGBA{R: 0x9A, G: 0xC6, B: 0xFF, A: 0xFF})

	listY, listH, panelW := t.layout()

	margin := 6
	listX := margin
	listW := t.w - 2*margin
	if panelW > 0 {
		listW = t.w - panelW - 3*margin
	}
	if listW < 0 {
		listW = 0
	}

	border := rgb565From888(0x2B, 0x33, 0x44)
	drawRectOutlineRGB565(buf, t.fb.StrideBytes(), listX, listY, listW, listH, border)
	t.renderList(listX, listY, listW, listH)

	if panelW > 0 {
		panelX := listX + listW + margin
		panelY := listY
		panelH := listH
		drawRectOutlineRGB565(buf, t.fb.StrideBytes(), panelX, panelY, panelW, panelH, border)
		t.renderPanel(panelX, panelY, panelW, panelH)
	}

	if t.status != "" {
		statusY := t.h - (int(t.fontHeight) + 6)
		if t.inputMode != inputNone {
			statusY -= int(t.fontHeight) + 6
		}
		if statusY > listY+listH {
			t.drawText(6, statusY, truncateToWidth(t.font, t.status, t.w-12), color.RGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xFF})
		}
	}

	if t.inputMode != inputNone {
		t.renderInputBar()
	}

	_ = t.fb.Present()
}

func (t *Task) statsLine() string {
	open := 0
	done := 0
	for i := 0; i < len(t.items); i++ {
		if t.items[i].done {
			done++
		} else {
			open++
		}
	}
	f := "all"
	switch t.filter {
	case filterOpen:
		f = "open"
	case filterDone:
		f = "done"
	}
	if t.search != "" {
		return fmt.Sprintf("open=%d done=%d filter=%s search=%q", open, done, f, t.search)
	}
	return fmt.Sprintf("open=%d done=%d filter=%s", open, done, f)
}

func (t *Task) renderList(x, y, w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	buf := t.fb.Buffer()
	if buf == nil {
		return
	}

	lineH := int(t.fontHeight) + 2
	rows := h / lineH
	if rows < 1 {
		rows = 1
	}

	if len(t.visible) == 0 {
		t.drawText(x+4, y+2, "(empty) press 'a' to add", color.RGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xFF})
		return
	}

	maxTextW := w - 28
	if maxTextW < 0 {
		maxTextW = 0
	}

	for row := 0; row < rows; row++ {
		i := t.top + row
		if i >= len(t.visible) {
			break
		}
		itemIdx := t.visible[i]
		it := t.items[itemIdx]
		yy := y + 2 + row*lineH

		if i == t.sel {
			fillRectRGB565(buf, t.fb.StrideBytes(), x+1, yy-1, w-2, lineH, rgb565From888(0x1A, 0x2D, 0x44))
		}

		check := "[ ]"
		if it.done {
			check = "[x]"
		}
		t.drawText(x+4, yy, check, color.RGBA{R: 0xEE, G: 0xEE, B: 0xEE, A: 0xFF})

		pc := color.RGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xFF}
		switch it.prio {
		case 0:
			pc = color.RGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xFF}
		case 1:
			pc = color.RGBA{R: 0xFF, G: 0xD1, B: 0x4A, A: 0xFF}
		case 2:
			pc = color.RGBA{R: 0xFF, G: 0x7F, B: 0x7F, A: 0xFF}
		}
		t.drawText(x+4+int(t.fontWidth)*3+4, yy, "!", pc)

		text := it.text
		if it.done {
			text = " " + text
		}
		t.drawText(x+4+int(t.fontWidth)*4+8, yy, truncateToWidth(t.font, text, maxTextW), color.RGBA{R: 0xD6, G: 0xD6, B: 0xD6, A: 0xFF})
	}
}

func (t *Task) renderPanel(x, y, w, h int) {
	maxTextW := w - 10
	if maxTextW < 0 {
		maxTextW = 0
	}
	lineH := int(t.fontHeight) + 2
	yy := y + 2

	header := "Help"
	t.drawText(x+4, yy, header, color.RGBA{R: 0x9A, G: 0xC6, B: 0xFF, A: 0xFF})
	yy += lineH

	for _, s := range []string{
		"Up/Down move",
		"Enter/Space toggle",
		"a add  e edit",
		"d delete  p prio",
		"/ search  f filter",
		"J/K reorder",
		"s save  q quit",
	} {
		if yy+int(t.fontHeight) >= y+h-2 {
			break
		}
		t.drawText(x+4, yy, truncateToWidth(t.font, s, maxTextW), color.RGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xFF})
		yy += lineH
	}

	idx, ok := t.selectedIndex()
	if !ok {
		return
	}
	it := t.items[idx]

	yy += lineH
	if yy+int(t.fontHeight) >= y+h-2 {
		return
	}
	t.drawText(x+4, yy, "Selected:", color.RGBA{R: 0xEE, G: 0xEE, B: 0xEE, A: 0xFF})
	yy += lineH

	for _, line := range wrapText(t.font, it.text, maxTextW) {
		if yy+int(t.fontHeight) >= y+h-2 {
			break
		}
		t.drawText(x+4, yy, line, color.RGBA{R: 0xD6, G: 0xD6, B: 0xD6, A: 0xFF})
		yy += lineH
	}
}

func (t *Task) renderInputBar() {
	buf := t.fb.Buffer()
	if buf == nil {
		return
	}
	barH := int(t.fontHeight) + 6
	y0 := t.h - barH
	fillRectRGB565(buf, t.fb.StrideBytes(), 0, y0, t.w, barH, rgb565From888(0x10, 0x14, 0x1E))
	drawRectOutlineRGB565(buf, t.fb.StrideBytes(), 0, y0, t.w, barH, rgb565From888(0x2B, 0x33, 0x44))

	prompt := t.inputPrompt + string(t.input)
	t.drawText(6, y0+3, truncateToWidth(t.font, prompt, t.w-12), color.RGBA{R: 0xEE, G: 0xEE, B: 0xEE, A: 0xFF})

	if (t.nowTick/350)%2 == 0 {
		cursorX := 6 + textWidth(t.font, t.inputPrompt) + textWidthRunes(t.font, t.input[:t.inputCursor])
		fillRectRGB565(buf, t.fb.StrideBytes(), cursorX, y0+3, 2, int(t.fontHeight), rgb565From888(0xFF, 0xFF, 0xFF))
	}
}

func truncateToWidth(f tinyfont.Fonter, s string, maxW int) string {
	if maxW <= 0 {
		return ""
	}
	w, _ := tinyfont.LineWidth(f, s)
	if int(w) <= maxW {
		return s
	}
	r := []rune(s)
	for len(r) > 0 {
		r = r[:len(r)-1]
		w, _ = tinyfont.LineWidth(f, string(r)+"…")
		if int(w) <= maxW {
			return string(r) + "…"
		}
	}
	return ""
}

func wrapText(f tinyfont.Fonter, s string, maxW int) []string {
	s = strings.TrimSpace(s)
	if s == "" || maxW <= 0 {
		return nil
	}
	if int(textWidth(f, s)) <= maxW {
		return []string{s}
	}

	words := strings.Fields(s)
	var out []string

	cur := ""
	for _, word := range words {
		next := word
		if cur != "" {
			next = cur + " " + word
		}
		if int(textWidth(f, next)) <= maxW {
			cur = next
			continue
		}

		if cur != "" {
			out = append(out, cur)
			cur = ""
		}

		if int(textWidth(f, word)) <= maxW {
			cur = word
			continue
		}

		r := []rune(word)
		for len(r) > 0 {
			n := len(r)
			for n > 1 && int(textWidthRunes(f, r[:n])) > maxW {
				n--
			}
			if n <= 0 {
				break
			}
			out = append(out, string(r[:n]))
			r = r[n:]
		}
	}

	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func textWidth(f tinyfont.Fonter, s string) int {
	w, _ := tinyfont.LineWidth(f, s)
	return int(w)
}

func textWidthRunes(f tinyfont.Fonter, r []rune) int {
	if len(r) == 0 {
		return 0
	}
	w, _ := tinyfont.LineWidth(f, string(r))
	return int(w)
}

func (t *Task) drawText(x, y int, s string, c color.RGBA) {
	d := &fbDisplayer{fb: t.fb}
	tinyfont.WriteLine(d, t.font, int16(x), int16(y)+t.fontHeight, s, c)
}

type fbDisplayer struct {
	fb hal.Framebuffer
}

func (d *fbDisplayer) Size() (x, y int16) {
	if d.fb == nil {
		return 0, 0
	}
	return int16(d.fb.Width()), int16(d.fb.Height())
}

func (d *fbDisplayer) SetPixel(x, y int16, c color.RGBA) {
	if d.fb == nil || d.fb.Format() != hal.PixelFormatRGB565 {
		return
	}
	buf := d.fb.Buffer()
	if buf == nil {
		return
	}
	w := d.fb.Width()
	h := d.fb.Height()
	ix := int(x)
	iy := int(y)
	if ix < 0 || ix >= w || iy < 0 || iy >= h {
		return
	}
	pixel := rgb565From888(c.R, c.G, c.B)
	off := iy*d.fb.StrideBytes() + ix*2
	if off < 0 || off+1 >= len(buf) {
		return
	}
	buf[off] = byte(pixel)
	buf[off+1] = byte(pixel >> 8)
}

func (d *fbDisplayer) Display() error { return nil }

func clearRGB565(buf []byte, pixel uint16) {
	lo := byte(pixel)
	hi := byte(pixel >> 8)
	for i := 0; i+1 < len(buf); i += 2 {
		buf[i] = lo
		buf[i+1] = hi
	}
}

func fillRectRGB565(buf []byte, stride, x0, y0, w, h int, pixel uint16) {
	if w <= 0 || h <= 0 {
		return
	}
	lo := byte(pixel)
	hi := byte(pixel >> 8)
	for y := 0; y < h; y++ {
		row := (y0+y)*stride + x0*2
		for x := 0; x < w; x++ {
			off := row + x*2
			if off < 0 || off+1 >= len(buf) {
				continue
			}
			buf[off] = lo
			buf[off+1] = hi
		}
	}
}

func drawRectOutlineRGB565(buf []byte, stride, x0, y0, w, h int, pixel uint16) {
	if w <= 0 || h <= 0 {
		return
	}
	drawHLineRGB565(buf, stride, x0, x0+w-1, y0, pixel)
	drawHLineRGB565(buf, stride, x0, x0+w-1, y0+h-1, pixel)
	drawVLineRGB565(buf, stride, x0, y0, y0+h-1, pixel)
	drawVLineRGB565(buf, stride, x0+w-1, y0, y0+h-1, pixel)
}

func drawHLineRGB565(buf []byte, stride, x0, x1, y int, pixel uint16) {
	if y < 0 || stride <= 0 {
		return
	}
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	lo := byte(pixel)
	hi := byte(pixel >> 8)
	row := y * stride
	for x := x0; x <= x1; x++ {
		off := row + x*2
		if off < 0 || off+1 >= len(buf) {
			continue
		}
		buf[off] = lo
		buf[off+1] = hi
	}
}

func drawVLineRGB565(buf []byte, stride, x, y0, y1 int, pixel uint16) {
	if x < 0 || stride <= 0 {
		return
	}
	if y0 > y1 {
		y0, y1 = y1, y0
	}
	lo := byte(pixel)
	hi := byte(pixel >> 8)
	for y := y0; y <= y1; y++ {
		off := y*stride + x*2
		if off < 0 || off+1 >= len(buf) {
			continue
		}
		buf[off] = lo
		buf[off+1] = hi
	}
}

func rgb565From888(r, g, b uint8) uint16 {
	return uint16((uint16(r>>3)&0x1F)<<11 | (uint16(g>>2)&0x3F)<<5 | (uint16(b>>3) & 0x1F))
}
