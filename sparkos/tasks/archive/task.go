package archive

import (
	"errors"
	"fmt"
	"image/color"
	"sort"
	"strings"

	"spark/hal"
	vfsclient "spark/sparkos/client/vfs"
	"spark/sparkos/fonts/font6x8cp1251"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"

	"tinygo.org/x/tinyfont"
)

type inputMode uint8

const (
	inputNone inputMode = iota
	inputOpen
	inputExtract
	inputCreate
)

type viewItemType uint8

const (
	viewDir viewItemType = iota
	viewFile
)

type viewItem struct {
	typ  viewItemType
	name string

	// For files only.
	entryIdx int
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

	archivePath string
	kind        archiveKind
	archiveSize uint32
	entries     []entry

	prefix string
	items  []viewItem

	sel int
	top int

	status string

	inputMode   inputMode
	inputPrompt string
	input       []rune
	inputCursor int

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
				if !ok || appID != proto.AppArchive {
					continue
				}
				if t.active {
					t.applyArg(ctx, arg)
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
	t.prefix = ""
	t.entries = nil
	t.items = nil
	t.sel = 0
	t.top = 0

	if t.archivePath == "" {
		t.beginOpen()
	}

	t.render()
}

func (t *Task) unload() {
	t.active = false
	t.entries = nil
	t.items = nil
	t.input = nil
	t.inbuf = nil
}

func (t *Task) requestExit(ctx *kernel.Context) {
	t.active = false
	if !t.muxCap.Valid() {
		return
	}
	_ = ctx.SendToCapRetry(t.muxCap, uint16(proto.MsgAppControl), proto.AppControlPayload(false), kernel.Capability{}, 500)
}

func (t *Task) applyArg(ctx *kernel.Context, arg string) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return
	}
	if strings.HasPrefix(arg, "open ") {
		t.openArchive(ctx, strings.TrimSpace(strings.TrimPrefix(arg, "open ")))
		return
	}
	t.openArchive(ctx, arg)
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
		t.activateSelected()
	case keyBackspace:
		t.goUp()
	case keyRune:
		t.handleRune(ctx, k.r)
	}
}

func (t *Task) handleRune(ctx *kernel.Context, r rune) {
	switch r {
	case 'q':
		t.requestExit(ctx)
	case 'o':
		t.beginOpen()
	case 'r':
		if t.archivePath != "" {
			t.openArchive(ctx, t.archivePath)
		}
	case 'x':
		t.beginExtract()
	case 'X':
		t.beginExtractAll()
	case 'c':
		t.beginCreate()
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
		s := strings.TrimSpace(string(t.input))
		mode := t.inputMode
		t.inputMode = inputNone
		t.inputPrompt = ""
		t.input = t.input[:0]
		t.inputCursor = 0
		t.applyInput(ctx, mode, s)
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
		if len(t.input) >= 200 {
			return
		}
		t.input = append(t.input, 0)
		copy(t.input[t.inputCursor+1:], t.input[t.inputCursor:])
		t.input[t.inputCursor] = k.r
		t.inputCursor++
	}
}

func (t *Task) applyInput(ctx *kernel.Context, mode inputMode, s string) {
	switch mode {
	case inputOpen:
		t.openArchive(ctx, s)
	case inputExtract:
		if s == "" {
			t.status = "Empty destination."
			return
		}
		if err := t.extractSelected(ctx, s); err != nil {
			t.status = "Extract: " + err.Error()
			return
		}
		t.status = "Extracted."
	case inputCreate:
		if err := t.createFromInput(ctx, s); err != nil {
			t.status = "Create: " + err.Error()
			return
		}
		t.status = "Created."
	}
}

func (t *Task) beginOpen() {
	t.inputMode = inputOpen
	t.inputPrompt = "Open archive path: "
	t.input = t.input[:0]
	t.inputCursor = 0
}

func (t *Task) beginExtract() {
	if t.archivePath == "" || len(t.items) == 0 {
		t.status = "Nothing to extract."
		return
	}
	t.inputMode = inputExtract
	t.inputPrompt = "Extract to dir (e.g. /): "
	t.input = t.input[:0]
	t.inputCursor = 0
}

func (t *Task) beginExtractAll() {
	if t.archivePath == "" {
		t.status = "No archive."
		return
	}
	t.sel = 0
	t.prefix = ""
	t.rebuildItems()
	t.inputMode = inputExtract
	t.inputPrompt = "Extract ALL to dir (e.g. /): "
	t.input = t.input[:0]
	t.inputCursor = 0
}

func (t *Task) beginCreate() {
	t.inputMode = inputCreate
	t.inputPrompt = "Create: <tar|zip> <out> <srcDir>: "
	t.input = t.input[:0]
	t.inputCursor = 0
}

func (t *Task) openArchive(ctx *kernel.Context, path string) {
	if t.vfs == nil {
		t.status = "VFS unavailable."
		return
	}
	path = strings.TrimSpace(path)
	if path == "" {
		t.status = "Empty path."
		return
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	typ, size, err := t.vfs.Stat(ctx, path)
	if err != nil || typ != proto.VFSEntryFile {
		t.status = "Not a file."
		return
	}
	head, _, err := t.readAtFull(ctx, path, 0, 64)
	if err != nil {
		t.status = "Read failed."
		return
	}

	kind := detectArchiveKind(path, head)
	if kind == archiveTarGz {
		t.status = "tar.gz not supported yet."
		return
	}

	readAt := func(off uint32, n uint16) ([]byte, bool, error) { return t.readAtFull(ctx, path, off, n) }

	var entries []entry
	switch kind {
	case archiveTar:
		entries, err = parseTarIndex(size, readAt)
	case archiveZip:
		entries, err = parseZipIndex(size, readAt)
	default:
		err = errors.New("unknown archive")
	}
	if err != nil {
		t.status = "Open: " + err.Error()
		return
	}

	t.archivePath = path
	t.kind = kind
	t.archiveSize = size
	t.entries = entries
	t.prefix = ""
	t.sel = 0
	t.top = 0
	t.rebuildItems()
	t.status = fmt.Sprintf("Loaded %s (%d entries).", kind.String(), len(entries))
}

func (t *Task) readAtFull(ctx *kernel.Context, path string, off uint32, n uint16) ([]byte, bool, error) {
	if t.vfs == nil {
		return nil, false, errors.New("vfs unavailable")
	}
	if n == 0 {
		return nil, false, nil
	}

	const maxChunk = kernel.MaxMessageBytes - 11

	out := make([]byte, 0, int(n))
	cur := off
	remaining := int(n)
	eof := false

	for remaining > 0 {
		want := remaining
		if want > maxChunk {
			want = maxChunk
		}

		b, gotEOF, err := t.vfs.ReadAt(ctx, path, cur, uint16(want))
		if err != nil {
			return nil, false, err
		}
		if len(b) == 0 {
			return out, true, nil
		}

		out = append(out, b...)
		cur += uint32(len(b))
		remaining -= len(b)
		if gotEOF {
			eof = true
			break
		}
	}

	return out, eof, nil
}

func (t *Task) rebuildItems() {
	t.items = t.items[:0]
	pfx := t.prefix
	if pfx != "" && !strings.HasSuffix(pfx, "/") {
		pfx += "/"
	}

	dirs := make(map[string]bool)
	files := make([]viewItem, 0, 32)

	for i := 0; i < len(t.entries); i++ {
		name := t.entries[i].name
		if pfx != "" {
			if !strings.HasPrefix(name, pfx) {
				continue
			}
			name = strings.TrimPrefix(name, pfx)
		}
		if name == "" {
			continue
		}
		part := name
		rest := ""
		if j := strings.IndexByte(name, '/'); j >= 0 {
			part = name[:j]
			rest = name[j+1:]
		}
		if rest != "" {
			dirs[part] = true
			continue
		}
		if stringsHasSuffix(part, "/") || t.entries[i].typ == entryDir {
			dirs[strings.TrimSuffix(part, "/")] = true
			continue
		}
		files = append(files, viewItem{typ: viewFile, name: part, entryIdx: i})
	}

	dirNames := make([]string, 0, len(dirs))
	for k := range dirs {
		dirNames = append(dirNames, k)
	}
	sort.Strings(dirNames)
	for _, d := range dirNames {
		t.items = append(t.items, viewItem{typ: viewDir, name: d + "/"})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })
	t.items = append(t.items, files...)

	if t.sel >= len(t.items) {
		t.sel = len(t.items) - 1
		if t.sel < 0 {
			t.sel = 0
		}
	}
	if t.top > t.sel {
		t.top = t.sel
	}
}

func (t *Task) moveSel(delta int) {
	if len(t.items) == 0 {
		return
	}
	t.sel += delta
	if t.sel < 0 {
		t.sel = 0
	}
	if t.sel >= len(t.items) {
		t.sel = len(t.items) - 1
	}
	t.ensureSelVisible()
}

func (t *Task) ensureSelVisible() {
	_, _, listW, listH := t.layout()
	if listW <= 0 || listH <= 0 {
		return
	}
	lineH := int(t.fontHeight) + 2
	rows := listH / lineH
	if rows < 1 {
		rows = 1
	}
	if t.sel < t.top {
		t.top = t.sel
	}
	if t.sel >= t.top+rows {
		t.top = t.sel - rows + 1
	}
	if t.top < 0 {
		t.top = 0
	}
}

func (t *Task) activateSelected() {
	if t.sel < 0 || t.sel >= len(t.items) {
		return
	}
	it := t.items[t.sel]
	if it.typ == viewDir {
		t.prefix = t.prefix + it.name
		t.sel = 0
		t.top = 0
		t.rebuildItems()
		return
	}
	t.status = t.describeSelected()
}

func (t *Task) goUp() {
	if t.prefix == "" {
		return
	}
	p := strings.TrimSuffix(t.prefix, "/")
	if p == "" {
		t.prefix = ""
	} else {
		if i := strings.LastIndexByte(p, '/'); i >= 0 {
			t.prefix = p[:i+1]
		} else {
			t.prefix = ""
		}
	}
	t.sel = 0
	t.top = 0
	t.rebuildItems()
}

func (t *Task) describeSelected() string {
	if t.sel < 0 || t.sel >= len(t.items) {
		return ""
	}
	it := t.items[t.sel]
	if it.typ == viewDir {
		return "Dir: " + it.name
	}
	if it.entryIdx < 0 || it.entryIdx >= len(t.entries) {
		return ""
	}
	e := t.entries[it.entryIdx]
	if t.kind == archiveZip && e.typ == entryFile && e.compMethod != 0 {
		return fmt.Sprintf("%s (%s) [unsupported]", e.name, fmtBytes(e.size))
	}
	return fmt.Sprintf("%s (%s)", e.name, fmtBytes(e.size))
}

func (t *Task) extractSelected(ctx *kernel.Context, dstDir string) error {
	if t.vfs == nil {
		return errors.New("vfs unavailable")
	}
	dstDir = strings.TrimSpace(dstDir)
	if dstDir == "" {
		return errors.New("empty destination")
	}
	if !strings.HasPrefix(dstDir, "/") {
		dstDir = "/" + dstDir
	}

	if t.archivePath == "" {
		return errors.New("no archive")
	}
	if t.kind != archiveTar && t.kind != archiveZip {
		return errors.New("unsupported archive")
	}

	readAt := func(off uint32, n uint16) ([]byte, bool, error) {
		return t.vfs.ReadAt(ctx, t.archivePath, off, n)
	}

	var wantPrefix string
	var wantEntryIdx int = -1
	if t.sel >= 0 && t.sel < len(t.items) {
		it := t.items[t.sel]
		if it.typ == viewDir {
			wantPrefix = t.prefix + it.name
		} else {
			wantEntryIdx = it.entryIdx
		}
	}

	extractOne := func(e entry) error {
		rel := sanitizeRelPath(e.name)
		if rel == "" {
			return nil
		}
		if e.typ == entryDir {
			return t.ensureDir(ctx, joinPath(dstDir, rel))
		}
		if t.kind == archiveZip {
			if err := zipEntryIsSupported(e); err != nil {
				return err
			}
		}

		outPath := joinPath(dstDir, rel)
		if err := t.ensureParentDirs(ctx, outPath); err != nil {
			return err
		}

		w, err := t.vfs.OpenWriter(ctx, outPath, proto.VFSWriteTruncate)
		if err != nil {
			return err
		}
		defer func() { _, _ = w.Close() }()

		var toCopy uint32
		if t.kind == archiveTar {
			toCopy = e.size
		} else {
			toCopy = e.compSize
		}

		var off uint32 = e.dataOff
		const chunkMax = 768
		for toCopy > 0 {
			n := uint16(chunkMax)
			if toCopy < uint32(n) {
				n = uint16(toCopy)
			}
			b, _, err := readAt(off, n)
			if err != nil {
				return err
			}
			if len(b) == 0 {
				return errors.New("unexpected EOF")
			}
			if _, err := w.Write(b); err != nil {
				return err
			}
			off += uint32(len(b))
			toCopy -= uint32(len(b))
		}
		_, err = w.Close()
		return err
	}

	if wantEntryIdx >= 0 {
		if wantEntryIdx >= len(t.entries) {
			return errors.New("bad selection")
		}
		return extractOne(t.entries[wantEntryIdx])
	}
	if wantPrefix == "" {
		wantPrefix = t.prefix
	}

	for i := 0; i < len(t.entries); i++ {
		if wantPrefix != "" && !strings.HasPrefix(t.entries[i].name, wantPrefix) {
			continue
		}
		if err := extractOne(t.entries[i]); err != nil {
			return err
		}
	}
	return nil
}

func (t *Task) ensureDir(ctx *kernel.Context, path string) error {
	if t.vfs == nil {
		return errors.New("vfs unavailable")
	}
	path = strings.TrimRight(path, "/")
	if path == "" {
		path = "/"
	}
	typ, _, err := t.vfs.Stat(ctx, path)
	if err == nil && typ == proto.VFSEntryDir {
		return nil
	}
	if err := t.vfs.Mkdir(ctx, path); err == nil {
		return nil
	}
	typ, _, err = t.vfs.Stat(ctx, path)
	if err != nil {
		return err
	}
	if typ != proto.VFSEntryDir {
		return errors.New("not a directory")
	}
	return nil
}

func (t *Task) ensureParentDirs(ctx *kernel.Context, path string) error {
	path = strings.TrimSpace(path)
	if path == "" || path == "/" {
		return nil
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	i := strings.LastIndexByte(path, '/')
	if i <= 0 {
		return nil
	}
	dir := path[:i]
	return t.ensureDirRecursive(ctx, dir)
}

func (t *Task) ensureDirRecursive(ctx *kernel.Context, dir string) error {
	dir = strings.TrimRight(dir, "/")
	if dir == "" {
		dir = "/"
	}
	if dir == "/" {
		return nil
	}
	parts := strings.Split(dir, "/")
	cur := ""
	for _, p := range parts {
		if p == "" {
			continue
		}
		cur += "/" + p
		if err := t.ensureDir(ctx, cur); err != nil {
			return err
		}
	}
	return nil
}

func (t *Task) createFromInput(ctx *kernel.Context, s string) error {
	if t.vfs == nil {
		return errors.New("vfs unavailable")
	}
	fields := strings.Fields(s)
	if len(fields) != 3 {
		return errors.New("expected: <tar|zip> <out> <srcDir>")
	}
	kindStr := strings.ToLower(fields[0])
	out := fields[1]
	src := fields[2]
	if !strings.HasPrefix(out, "/") {
		out = "/" + out
	}
	if !strings.HasPrefix(src, "/") {
		src = "/" + src
	}

	switch kindStr {
	case "tar":
		return t.createTar(ctx, out, src)
	case "zip":
		return t.createZipStore(ctx, out, src)
	default:
		return errors.New("unsupported kind (use tar or zip)")
	}
}

func (t *Task) createTar(ctx *kernel.Context, outPath, srcDir string) error {
	typ, _, err := t.vfs.Stat(ctx, srcDir)
	if err != nil || typ != proto.VFSEntryDir {
		return errors.New("srcDir is not a directory")
	}
	if err := t.ensureParentDirs(ctx, outPath); err != nil {
		return err
	}
	w, err := t.vfs.OpenWriter(ctx, outPath, proto.VFSWriteTruncate)
	if err != nil {
		return err
	}
	defer func() { _, _ = w.Close() }()

	addFile := func(rel string, filePath string, size uint32) error {
		hdr := tarHeader(rel, size, false)
		if _, err := w.Write(hdr[:]); err != nil {
			return err
		}
		var off uint32
		remain := size
		for remain > 0 {
			n := uint16(768)
			if remain < uint32(n) {
				n = uint16(remain)
			}
			b, _, err := t.vfs.ReadAt(ctx, filePath, off, n)
			if err != nil {
				return err
			}
			if len(b) == 0 {
				return errors.New("unexpected EOF")
			}
			if _, err := w.Write(b); err != nil {
				return err
			}
			off += uint32(len(b))
			remain -= uint32(len(b))
		}
		pad := int(roundUp512(size) - size)
		if pad > 0 {
			var zeros [tarBlockSize]byte
			if _, err := w.Write(zeros[:pad]); err != nil {
				return err
			}
		}
		return nil
	}

	addDir := func(rel string) error {
		hdr := tarHeader(rel, 0, true)
		_, err := w.Write(hdr[:])
		return err
	}

	if err := t.walkDir(ctx, srcDir, "", func(rel, full string, typ proto.VFSEntryType, size uint32) error {
		switch typ {
		case proto.VFSEntryDir:
			if rel != "" {
				return addDir(rel)
			}
		case proto.VFSEntryFile:
			return addFile(rel, full, size)
		}
		return nil
	}); err != nil {
		return err
	}

	var zeros [tarBlockSize]byte
	if _, err := w.Write(zeros[:]); err != nil {
		return err
	}
	if _, err := w.Write(zeros[:]); err != nil {
		return err
	}
	_, err = w.Close()
	return err
}

func (t *Task) createZipStore(ctx *kernel.Context, outPath, srcDir string) error {
	typ, _, err := t.vfs.Stat(ctx, srcDir)
	if err != nil || typ != proto.VFSEntryDir {
		return errors.New("srcDir is not a directory")
	}
	if err := t.ensureParentDirs(ctx, outPath); err != nil {
		return err
	}
	w, err := t.vfs.OpenWriter(ctx, outPath, proto.VFSWriteTruncate)
	if err != nil {
		return err
	}
	defer func() { _, _ = w.Close() }()

	zw := newZipStoreWriter(w)
	if err := t.walkDir(ctx, srcDir, "", func(rel, full string, typ proto.VFSEntryType, size uint32) error {
		switch typ {
		case proto.VFSEntryDir:
			if rel == "" {
				return nil
			}
			return zw.AddDir(rel)
		case proto.VFSEntryFile:
			return zw.AddFile(ctx, t.vfs, rel, full, size)
		default:
			return nil
		}
	}); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	_, err = w.Close()
	return err
}

func (t *Task) walkDir(
	ctx *kernel.Context,
	dir string,
	relBase string,
	visit func(rel, full string, typ proto.VFSEntryType, size uint32) error,
) error {
	entries, err := t.vfs.List(ctx, dir)
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })

	for _, ent := range entries {
		if ent.Name == "" || ent.Name == "." || ent.Name == ".." {
			continue
		}
		full := dir
		if full == "/" {
			full = "/" + ent.Name
		} else {
			full = full + "/" + ent.Name
		}
		rel := ent.Name
		if relBase != "" {
			rel = relBase + "/" + ent.Name
		}
		rel = sanitizeRelPath(rel)

		if err := visit(rel, full, ent.Type, ent.Size); err != nil {
			return err
		}
		if ent.Type == proto.VFSEntryDir {
			if err := t.walkDir(ctx, full, rel, visit); err != nil {
				return err
			}
		}
	}
	return nil
}

func (t *Task) layout() (top int, listY int, listW int, listH int) {
	top = int(t.fontHeight)*2 + 10
	bottomReserved := int(t.fontHeight) + 4
	if t.inputMode != inputNone {
		bottomReserved = int(t.fontHeight) + 6
	}
	listY = top
	listW = t.w - 12
	listH = t.h - top - bottomReserved - 6
	if listH < 0 {
		listH = 0
	}
	return top, listY, listW, listH
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

	title := "ARCHIVE"
	sub := t.archivePath
	if sub == "" {
		sub = "(no archive)  press 'o' to open"
	} else {
		sub = fmt.Sprintf("%s  [%s]  %s", t.archivePath, t.kind.String(), t.prefix)
	}
	t.drawText(6, 0, title, color.RGBA{R: 0xEE, G: 0xEE, B: 0xEE, A: 0xFF})
	t.drawText(6, int(t.fontHeight)+1, truncateToWidth(t.font, sub, t.w-12), color.RGBA{R: 0x9A, G: 0xC6, B: 0xFF, A: 0xFF})

	_, listY, listW, listH := t.layout()
	border := rgb565From888(0x2B, 0x33, 0x44)
	drawRectOutlineRGB565(buf, t.fb.StrideBytes(), 6, listY, listW, listH, border)
	t.renderList(6, listY, listW, listH)

	if t.status != "" {
		statusY := t.h - (int(t.fontHeight) + 4)
		if t.inputMode != inputNone {
			statusY -= int(t.fontHeight) + 6
		}
		if statusY > listY+listH {
			t.drawText(6, statusY, truncateToWidth(t.font, t.status, t.w-12), color.RGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xFF})
		}
	}

	t.renderFooter()
	if t.inputMode != inputNone {
		t.renderInputBar()
	}

	_ = t.fb.Present()
}

func (t *Task) renderFooter() {
	buf := t.fb.Buffer()
	if buf == nil {
		return
	}
	h := int(t.fontHeight) + 6
	y0 := t.h - h
	if t.inputMode != inputNone {
		y0 -= h
	}
	fillRectRGB565(buf, t.fb.StrideBytes(), 0, y0, t.w, h, rgb565From888(0x10, 0x14, 0x1E))
	drawRectOutlineRGB565(buf, t.fb.StrideBytes(), 0, y0, t.w, h, rgb565From888(0x2B, 0x33, 0x44))
	help := "Up/Down select  Enter open  Backspace up  x extract  c create  o open  q quit"
	t.drawText(6, y0+3, truncateToWidth(t.font, help, t.w-12), color.RGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xFF})
}

func (t *Task) renderInputBar() {
	buf := t.fb.Buffer()
	if buf == nil {
		return
	}
	h := int(t.fontHeight) + 6
	y0 := t.h - h
	fillRectRGB565(buf, t.fb.StrideBytes(), 0, y0, t.w, h, rgb565From888(0x10, 0x14, 0x1E))
	drawRectOutlineRGB565(buf, t.fb.StrideBytes(), 0, y0, t.w, h, rgb565From888(0x2B, 0x33, 0x44))

	prompt := t.inputPrompt + string(t.input)
	t.drawText(6, y0+3, truncateToWidth(t.font, prompt, t.w-12), color.RGBA{R: 0xEE, G: 0xEE, B: 0xEE, A: 0xFF})
	if (t.nowTick/350)%2 == 0 {
		cursorX := 6 + textWidth(t.font, t.inputPrompt) + textWidthRunes(t.font, t.input[:t.inputCursor])
		fillRectRGB565(buf, t.fb.StrideBytes(), cursorX, y0+3, 2, int(t.fontHeight), rgb565From888(0xFF, 0xFF, 0xFF))
	}
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
	if len(t.items) == 0 {
		t.drawText(x+4, y+2, "(empty)", color.RGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xFF})
		return
	}

	maxTextW := w - 10
	for row := 0; row < rows; row++ {
		i := t.top + row
		if i >= len(t.items) {
			break
		}
		it := t.items[i]
		yy := y + 2 + row*lineH
		if i == t.sel {
			fillRectRGB565(buf, t.fb.StrideBytes(), x+1, yy-1, w-2, lineH, rgb565From888(0x1A, 0x2D, 0x44))
		}
		label := it.name
		if it.typ == viewFile && it.entryIdx >= 0 && it.entryIdx < len(t.entries) {
			e := t.entries[it.entryIdx]
			label = fmt.Sprintf("%s  %s", it.name, fmtBytes(e.size))
		}
		t.drawText(x+4, yy, truncateToWidth(t.font, label, maxTextW), color.RGBA{R: 0xD6, G: 0xD6, B: 0xD6, A: 0xFF})
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
