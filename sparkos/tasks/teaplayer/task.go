package teaplayer

import (
	"errors"
	"fmt"
	"image/color"
	"sort"
	"strings"
	"unicode/utf8"

	"spark/hal"
	audioclient "spark/sparkos/client/audio"
	vfsclient "spark/sparkos/client/vfs"
	"spark/sparkos/fonts/font6x8cp1251"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"

	"tinygo.org/x/tinyfont"
)

type entry struct {
	name string
	typ  proto.VFSEntryType
	size uint32
}

type Task struct {
	disp     hal.Display
	ep       kernel.Capability
	vfsCap   kernel.Capability
	audioCap kernel.Capability

	fb hal.Framebuffer

	font       tinyfont.Fonter
	fontWidth  int16
	fontHeight int16

	active bool
	muxCap kernel.Capability

	w int
	h int

	nowTick uint64

	vfs   *vfsclient.Client
	audio *audioclient.Client

	statusEP      kernel.Capability
	statusCapXfer kernel.Capability
	statusCh      <-chan kernel.Message

	cwd   string
	items []entry
	sel   int
	top   int

	loop bool

	nowState      proto.AudioState
	nowVolume     uint8
	nowSampleRate uint16
	nowPos        uint32
	nowTotal      uint32

	eq [8]uint8

	nowPath string

	status string

	inbuf []byte
}

func New(disp hal.Display, ep kernel.Capability, vfsCap kernel.Capability, audioCap kernel.Capability) *Task {
	return &Task{disp: disp, ep: ep, vfsCap: vfsCap, audioCap: audioCap}
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
				if !ok || appID != proto.AppTEA {
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

		case msg, ok := <-t.statusCh:
			if !ok {
				t.statusCh = nil
				continue
			}
			if !t.active {
				continue
			}
			switch proto.Kind(msg.Kind) {
			case proto.MsgAudioStatus:
				state, vol, sr, pos, total, ok := proto.DecodeAudioStatusPayload(msg.Payload())
				if !ok {
					continue
				}
				t.nowState = state
				t.nowVolume = vol
				t.nowSampleRate = sr
				t.nowPos = pos
				t.nowTotal = total
				t.render()
			case proto.MsgAudioMeters:
				levels, ok := proto.DecodeAudioMetersPayload(msg.Payload())
				if !ok {
					continue
				}
				for i := range t.eq {
					t.eq[i] = 0
				}
				for i := 0; i < len(levels) && i < len(t.eq); i++ {
					t.eq[i] = levels[i]
				}
				t.render()
			}

		case now := <-tickCh:
			if !t.active {
				continue
			}
			t.nowTick = now
			if t.nowState == proto.AudioPlaying && (now/250)%2 == 0 {
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

	if t.cwd == "" {
		t.cwd = "/"
	}
	if t.vfs == nil && t.vfsCap.Valid() {
		t.vfs = vfsclient.New(t.vfsCap)
	}
	if t.audio == nil && t.audioCap.Valid() {
		t.audio = audioclient.New(t.audioCap)
	}

	if t.statusCh == nil && t.audio != nil {
		ep := ctx.NewEndpoint(kernel.RightSend | kernel.RightRecv)
		if ep.Valid() {
			ch, ok := ctx.RecvChan(ep.Restrict(kernel.RightRecv))
			if ok {
				t.statusEP = ep
				t.statusCapXfer = ep.Restrict(kernel.RightSend)
				t.statusCh = ch
				_ = t.audio.Subscribe(ctx, t.statusCapXfer)
			}
		}
	}

	t.refreshList(ctx)
}

func (t *Task) unload() {
	t.active = false
	t.items = nil
	t.inbuf = nil
}

func (t *Task) requestExit(ctx *kernel.Context) {
	t.active = false
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

func (t *Task) applyArg(ctx *kernel.Context, arg string) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return
	}
	if strings.HasSuffix(strings.ToLower(arg), ".tea") {
		t.nowPath = arg
		t.cwd = parentDir(arg)
		t.refreshList(ctx)
		t.selectName(lastPathElem(arg))
		_ = t.playSelected(ctx)
		return
	}
	t.cwd = arg
	t.refreshList(ctx)
}

func (t *Task) refreshList(ctx *kernel.Context) {
	t.status = ""
	t.items = nil
	t.sel = 0
	t.top = 0

	if t.vfs == nil {
		t.status = "vfs unavailable"
		return
	}

	entries, err := t.vfs.List(ctx, t.cwd)
	if err != nil {
		t.status = err.Error()
		return
	}

	var out []entry
	if t.cwd != "/" {
		out = append(out, entry{name: "..", typ: proto.VFSEntryDir})
	}

	for _, e := range entries {
		if e.Name == "." || e.Name == ".." {
			continue
		}
		if e.Type == proto.VFSEntryDir {
			out = append(out, entry{name: e.Name, typ: e.Type, size: e.Size})
			continue
		}
		if e.Type == proto.VFSEntryFile && strings.HasSuffix(strings.ToLower(e.Name), ".tea") {
			out = append(out, entry{name: e.Name, typ: e.Type, size: e.Size})
		}
	}

	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.typ != b.typ {
			return a.typ == proto.VFSEntryDir
		}
		return strings.ToLower(a.name) < strings.ToLower(b.name)
	})
	t.items = out
}

func (t *Task) selectName(name string) {
	for i := range t.items {
		if t.items[i].name == name {
			t.sel = i
			t.ensureVisible()
			return
		}
	}
}

func (t *Task) ensureVisible() {
	if t.sel < t.top {
		t.top = t.sel
		return
	}
	listH := t.h - int(t.fontHeight)*3 - 18
	lineH := int(t.fontHeight) + 2
	rows := listH / lineH
	if rows < 1 {
		rows = 1
	}
	if t.sel >= t.top+rows {
		t.top = t.sel - rows + 1
	}
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

type keyKind uint8

const (
	keyNone keyKind = iota
	keyEsc
	keyEnter
	keyBackspace
	keyUp
	keyDown
	keyRune
)

type key struct {
	kind keyKind
	r    rune
}

func nextKey(b []byte) (consumed int, k key, ok bool) {
	if len(b) == 0 {
		return 0, key{}, false
	}

	switch b[0] {
	case 0x1b:
		if len(b) == 1 {
			return 1, key{kind: keyEsc}, true
		}
		if len(b) < 3 {
			return 0, key{}, false
		}
		if b[1] != '[' {
			return 1, key{kind: keyEsc}, true
		}
		switch b[2] {
		case 'A':
			return 3, key{kind: keyUp}, true
		case 'B':
			return 3, key{kind: keyDown}, true
		default:
			return 1, key{kind: keyEsc}, true
		}
	case '\r', '\n':
		return 1, key{kind: keyEnter}, true
	case 0x7f, 0x08:
		return 1, key{kind: keyBackspace}, true
	}

	if !utf8.FullRune(b) {
		return 0, key{}, false
	}
	r, sz := utf8.DecodeRune(b)
	if r == utf8.RuneError && sz == 1 {
		return 1, key{}, true
	}
	if r < 0x20 {
		return sz, key{}, true
	}
	return sz, key{kind: keyRune, r: r}, true
}

func (t *Task) handleKey(ctx *kernel.Context, k key) {
	switch k.kind {
	case keyEsc:
		t.requestExit(ctx)
	case keyUp:
		t.moveSel(-1)
	case keyDown:
		t.moveSel(1)
	case keyEnter:
		t.activateSelected(ctx)
	case keyBackspace:
		t.goUp(ctx)
	case keyRune:
		t.handleRune(ctx, k.r)
	}
}

func (t *Task) handleRune(ctx *kernel.Context, r rune) {
	switch r {
	case 'q':
		t.requestExit(ctx)
	case ' ':
		if t.audio != nil {
			_ = t.audio.Pause(ctx)
		}
	case 's':
		if t.audio != nil {
			_ = t.audio.Stop(ctx)
		}
	case 'l':
		t.loop = !t.loop
	case '+', '=':
		t.bumpVolume(ctx, 8)
	case '-', '_':
		t.bumpVolume(ctx, -8)
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
	t.ensureVisible()
}

func (t *Task) goUp(ctx *kernel.Context) {
	if t.cwd == "/" {
		return
	}
	parent := parentDir(t.cwd)
	if parent == "" {
		parent = "/"
	}
	base := lastPathElem(t.cwd)
	t.cwd = parent
	t.refreshList(ctx)
	t.selectName(base)
}

func (t *Task) activateSelected(ctx *kernel.Context) {
	if len(t.items) == 0 || t.sel < 0 || t.sel >= len(t.items) {
		return
	}
	it := t.items[t.sel]
	if it.typ == proto.VFSEntryDir {
		if it.name == ".." {
			t.goUp(ctx)
			return
		}
		t.cwd = joinPath(t.cwd, it.name)
		t.refreshList(ctx)
		return
	}
	_ = t.playSelected(ctx)
}

func (t *Task) playSelected(ctx *kernel.Context) error {
	if t.audio == nil {
		t.status = "audio unavailable"
		return errors.New(t.status)
	}
	if len(t.items) == 0 || t.sel < 0 || t.sel >= len(t.items) {
		return nil
	}
	it := t.items[t.sel]
	if it.typ != proto.VFSEntryFile {
		return nil
	}
	full := joinPath(t.cwd, it.name)
	t.nowPath = full
	t.status = "play: " + it.name
	return t.audio.Play(ctx, full, t.loop)
}

func (t *Task) bumpVolume(ctx *kernel.Context, delta int) {
	if t.audio == nil {
		return
	}
	v := int(t.nowVolume) + delta
	if v < 0 {
		v = 0
	}
	if v > 255 {
		v = 255
	}
	_ = t.audio.SetVolume(ctx, uint8(v))
}

func (t *Task) render() {
	if !t.active || t.fb == nil || t.fb.Format() != hal.PixelFormatRGB565 {
		return
	}
	buf := t.fb.Buffer()
	if buf == nil {
		return
	}

	clearRGB565(buf, rgb565From888(0x09, 0x0B, 0x10))

	pad := 8
	titleH := int(t.fontHeight)*2 + 14
	footerH := int(t.fontHeight)*3 + 10

	listW := (t.w - pad*3) / 2
	if listW < 140 {
		listW = 140
	}
	if listW > t.w-pad*3-140 {
		listW = t.w - pad*3 - 140
	}

	x0 := pad
	y0 := pad
	xList := x0
	yList := y0 + titleH
	contentH := t.h - yList - footerH - pad*2
	if contentH < 0 {
		contentH = 0
	}

	xInfo := xList + listW + pad
	wInfo := t.w - xInfo - pad

	t.drawText(pad, pad+2, "TEA PLAYER", color.RGBA{R: 0xEE, G: 0xEE, B: 0xEE, A: 0xFF})
	t.drawText(pad, pad+2+int(t.fontHeight)+4, truncateToWidth(t.font, "Dir: "+t.cwd, t.w-pad*2), color.RGBA{R: 0x88, G: 0xA6, B: 0xD6, A: 0xFF})

	drawRectOutlineRGB565(buf, t.fb.StrideBytes(), xList, yList, listW, contentH, rgb565From888(0x2B, 0x33, 0x44))
	t.renderList(xList+1, yList+1, listW-2, contentH-2)

	// Split the right panel vertically: info top, EQ bottom.
	splitGap := pad
	infoH := (contentH - splitGap) / 2
	if infoH < 0 {
		infoH = 0
	}
	eqH := contentH - splitGap - infoH
	if eqH < 0 {
		eqH = 0
	}
	drawRectOutlineRGB565(buf, t.fb.StrideBytes(), xInfo, yList, wInfo, infoH, rgb565From888(0x2B, 0x33, 0x44))
	t.renderInfo(xInfo+1, yList+1, wInfo-2, infoH-2)
	yEQ := yList + infoH + splitGap
	drawRectOutlineRGB565(buf, t.fb.StrideBytes(), xInfo, yEQ, wInfo, eqH, rgb565From888(0x2B, 0x33, 0x44))
	t.renderEQ(xInfo+1, yEQ+1, wInfo-2, eqH-2)
	t.renderFooter(pad, t.h-footerH-pad, t.w-pad*2, footerH)

	_ = t.fb.Present()
}

func (t *Task) renderEQ(x, y, w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	buf := t.fb.Buffer()
	if buf == nil {
		return
	}

	fillRectRGB565(buf, t.fb.StrideBytes(), x, y, w, h, rgb565From888(0x10, 0x14, 0x1E))
	drawRectOutlineRGB565(buf, t.fb.StrideBytes(), x, y, w, h, rgb565From888(0x2B, 0x33, 0x44))
	t.drawText(x+6, y+3, "EQ", color.RGBA{R: 0x88, G: 0xA6, B: 0xD6, A: 0xFF})

	bars := len(t.eq)
	if bars == 0 {
		return
	}
	innerX := x + 30
	innerY := y + 3
	innerW := w - 36
	innerH := h - 8
	if innerW <= 0 || innerH <= 0 {
		return
	}

	gap := 2
	barW := (innerW - (bars-1)*gap) / bars
	if barW < 1 {
		barW = 1
	}

	for i := 0; i < bars; i++ {
		lvl := int(t.eq[i])
		barH := (innerH * lvl) / 255
		if barH < 0 {
			barH = 0
		}
		if barH > innerH {
			barH = innerH
		}
		bx := innerX + i*(barW+gap)
		by := innerY + (innerH - barH)
		fillRectRGB565(buf, t.fb.StrideBytes(), bx, by, barW, barH, rgb565From888(0x3A, 0x8B, 0xFF))
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
		t.drawText(x+6, y+6, "(no .tea files)", color.RGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xFF})
		return
	}

	maxTextW := w - 12
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
		prefix := "  "
		if it.typ == proto.VFSEntryDir {
			prefix = "[D]"
		}
		label := prefix + " " + it.name
		if it.typ == proto.VFSEntryFile && it.size > 0 {
			label = fmt.Sprintf("%s  %s", label, fmtBytes(it.size))
		}
		t.drawText(x+6, yy, truncateToWidth(t.font, label, maxTextW), color.RGBA{R: 0xD6, G: 0xD6, B: 0xD6, A: 0xFF})
	}
}

func (t *Task) renderInfo(x, y, w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	buf := t.fb.Buffer()
	if buf == nil {
		return
	}

	title := "Now playing"
	if t.nowPath == "" {
		title = "No track selected"
	}
	t.drawText(x+8, y+6, title, color.RGBA{R: 0xEE, G: 0xEE, B: 0xEE, A: 0xFF})

	yy := y + 6 + int(t.fontHeight) + 6
	if t.nowPath != "" {
		t.drawText(x+8, yy, truncateToWidth(t.font, lastPathElem(t.nowPath), w-16), color.RGBA{R: 0x88, G: 0xA6, B: 0xD6, A: 0xFF})
		yy += int(t.fontHeight) + 6
	}

	stateStr := "stopped"
	switch t.nowState {
	case proto.AudioPlaying:
		stateStr = "playing"
	case proto.AudioPaused:
		stateStr = "paused"
	}
	t.drawText(x+8, yy, fmt.Sprintf("State: %s", stateStr), color.RGBA{R: 0xD6, G: 0xD6, B: 0xD6, A: 0xFF})
	yy += int(t.fontHeight) + 4

	t.drawText(x+8, yy, fmt.Sprintf("Volume: %d", t.nowVolume), color.RGBA{R: 0xD6, G: 0xD6, B: 0xD6, A: 0xFF})
	yy += int(t.fontHeight) + 4

	if t.nowSampleRate > 0 {
		t.drawText(x+8, yy, fmt.Sprintf("Rate: %d Hz", t.nowSampleRate), color.RGBA{R: 0xD6, G: 0xD6, B: 0xD6, A: 0xFF})
		yy += int(t.fontHeight) + 6
	}

	if t.nowTotal > 0 && t.nowSampleRate > 0 {
		posS := int(t.nowPos) / int(t.nowSampleRate)
		totS := int(t.nowTotal) / int(t.nowSampleRate)
		t.drawText(x+8, yy, fmt.Sprintf("Time: %s / %s", fmtMMSS(posS), fmtMMSS(totS)), color.RGBA{R: 0xD6, G: 0xD6, B: 0xD6, A: 0xFF})
		yy += int(t.fontHeight) + 8

		barY := yy
		barH := int(t.fontHeight)
		fillRectRGB565(buf, t.fb.StrideBytes(), x+8, barY, w-16, barH, rgb565From888(0x12, 0x16, 0x20))
		drawRectOutlineRGB565(buf, t.fb.StrideBytes(), x+8, barY, w-16, barH, rgb565From888(0x2B, 0x33, 0x44))
		pct := float64(t.nowPos) / float64(t.nowTotal)
		if pct < 0 {
			pct = 0
		}
		if pct > 1 {
			pct = 1
		}
		fillW := int(float64(w-18) * pct)
		if fillW > 0 {
			fillRectRGB565(buf, t.fb.StrideBytes(), x+9, barY+1, fillW, barH-2, rgb565From888(0x3A, 0x8B, 0xFF))
		}
	}
}

func (t *Task) renderFooter(x, y, w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	buf := t.fb.Buffer()
	if buf == nil {
		return
	}
	fillRectRGB565(buf, t.fb.StrideBytes(), x, y, w, h, rgb565From888(0x10, 0x14, 0x1E))
	drawRectOutlineRGB565(buf, t.fb.StrideBytes(), x, y, w, h, rgb565From888(0x2B, 0x33, 0x44))

	line1 := "Enter play/open dir  Space pause  s stop  +/- vol  l loop  q quit"
	line2 := "Backspace up  ↑↓ select"
	line3 := t.status
	if t.loop {
		line2 += "  loop:on"
	} else {
		line2 += "  loop:off"
	}
	if line3 == "" && t.nowPath != "" {
		line3 = "Selected: " + lastPathElem(t.nowPath)
	}

	t.drawText(x+6, y+4, truncateToWidth(t.font, line1, w-12), color.RGBA{R: 0xD6, G: 0xD6, B: 0xD6, A: 0xFF})
	t.drawText(x+6, y+4+int(t.fontHeight)+2, truncateToWidth(t.font, line2, w-12), color.RGBA{R: 0x88, G: 0xA6, B: 0xD6, A: 0xFF})
	t.drawText(x+6, y+4+int(t.fontHeight)*2+4, truncateToWidth(t.font, line3, w-12), color.RGBA{R: 0xAA, G: 0xAA, B: 0xAA, A: 0xFF})
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
	for yy := 0; yy < h; yy++ {
		row := (y0+yy)*stride + x0*2
		for xx := 0; xx < w; xx++ {
			off := row + xx*2
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
	lo := byte(pixel)
	hi := byte(pixel >> 8)

	for x := 0; x < w; x++ {
		off0 := y0*stride + (x0+x)*2
		off1 := (y0+h-1)*stride + (x0+x)*2
		if off0 >= 0 && off0+1 < len(buf) {
			buf[off0] = lo
			buf[off0+1] = hi
		}
		if off1 >= 0 && off1+1 < len(buf) {
			buf[off1] = lo
			buf[off1+1] = hi
		}
	}
	for y := 0; y < h; y++ {
		off0 := (y0+y)*stride + x0*2
		off1 := (y0+y)*stride + (x0+w-1)*2
		if off0 >= 0 && off0+1 < len(buf) {
			buf[off0] = lo
			buf[off0+1] = hi
		}
		if off1 >= 0 && off1+1 < len(buf) {
			buf[off1] = lo
			buf[off1+1] = hi
		}
	}
}

func rgb565From888(r, g, b uint8) uint16 {
	rr := uint16(r>>3) & 0x1F
	gg := uint16(g>>2) & 0x3F
	bb := uint16(b>>3) & 0x1F
	return (rr << 11) | (gg << 5) | bb
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

func joinPath(dir, rel string) string {
	dir = strings.TrimSpace(dir)
	if dir == "" || dir == "/" {
		return "/" + strings.TrimLeft(rel, "/")
	}
	dir = strings.TrimRight(dir, "/")
	rel = strings.TrimLeft(rel, "/")
	if rel == "" {
		return dir
	}
	return dir + "/" + rel
}

func parentDir(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || path == "/" {
		return "/"
	}
	path = strings.TrimRight(path, "/")
	i := strings.LastIndex(path, "/")
	if i <= 0 {
		return "/"
	}
	return path[:i]
}

func lastPathElem(path string) string {
	path = strings.TrimRight(path, "/")
	i := strings.LastIndex(path, "/")
	if i < 0 {
		return path
	}
	return path[i+1:]
}

func fmtBytes(n uint32) string {
	if n < 1024 {
		return fmt.Sprintf("%dB", n)
	}
	kb := float64(n) / 1024.0
	if kb < 1024 {
		return fmt.Sprintf("%.1fKB", kb)
	}
	mb := kb / 1024.0
	if mb < 1024 {
		return fmt.Sprintf("%.1fMB", mb)
	}
	gb := mb / 1024.0
	return fmt.Sprintf("%.1fGB", gb)
}

func fmtMMSS(seconds int) string {
	if seconds < 0 {
		seconds = 0
	}
	m := seconds / 60
	s := seconds % 60
	return fmt.Sprintf("%02d:%02d", m, s)
}
