package snake

import (
	"fmt"
	"image/color"

	"spark/hal"
	"spark/sparkos/fonts/font6x8cp1251"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"

	"tinygo.org/x/tinyfont"
)

type dir uint8

const (
	dirUp dir = iota
	dirRight
	dirDown
	dirLeft
)

type point struct {
	x int
	y int
}

type Task struct {
	disp hal.Display
	ep   kernel.Capability

	fb hal.Framebuffer

	font       tinyfont.Fonter
	fontWidth  int16
	fontHeight int16

	cols int
	rows int

	active bool
	muxCap kernel.Capability

	cell  int
	gridW int
	gridH int

	snake   []point
	headDir dir
	nextDir dir

	food point
	rng  uint32

	score  int
	alive  bool
	paused bool

	lastStep uint64

	inbuf []byte
}

const (
	stepIntervalBaseTicks = 22
	stepIntervalMinTicks  = 8
)

func New(disp hal.Display, ep kernel.Capability) *Task {
	return &Task{
		disp: disp,
		ep:   ep,
		cell: 10,
	}
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

	t.cols = t.fb.Width() / int(t.fontWidth)
	t.rows = t.fb.Height() / int(t.fontHeight)
	if t.cols <= 0 || t.rows <= 0 {
		return
	}

	done := make(chan struct{})
	defer close(done)

	tickCh := make(chan uint64, 16)
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
		case msg := <-ch:
			switch proto.Kind(msg.Kind) {
			case proto.MsgAppShutdown:
				t.unload()
				return

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
				appID, _, ok := proto.DecodeAppSelectPayload(msg.Data[:msg.Len])
				if !ok || appID != proto.AppSnake {
					continue
				}
				if t.active {
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

		case now := <-tickCh:
			if !t.active || t.paused || !t.alive {
				continue
			}
			interval := uint64(t.stepIntervalTicks())
			if interval == 0 {
				interval = 1
			}
			if now-t.lastStep < interval {
				continue
			}
			t.lastStep = now
			t.step()
			t.render()
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

	t.initGame()
	t.lastStep = ctx.NowTick()
	t.render()
}

func (t *Task) unload() {
	t.active = false
	t.inbuf = nil
	t.snake = nil
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
	switch k.kind {
	case keyEsc:
		t.requestExit(ctx)
	case keyUp:
		t.setDir(ctx, dirUp)
	case keyDown:
		t.setDir(ctx, dirDown)
	case keyLeft:
		t.setDir(ctx, dirLeft)
	case keyRight:
		t.setDir(ctx, dirRight)
	case keyRune:
		switch k.r {
		case 'q':
			t.requestExit(ctx)
		case 'p', ' ':
			t.paused = !t.paused
		case 'r':
			t.initGame()
		}
	}
}

func (t *Task) setDir(ctx *kernel.Context, d dir) {
	if !t.alive {
		return
	}
	if (t.headDir == dirUp && d == dirDown) ||
		(t.headDir == dirDown && d == dirUp) ||
		(t.headDir == dirLeft && d == dirRight) ||
		(t.headDir == dirRight && d == dirLeft) {
		return
	}
	t.nextDir = d

	// Improve responsiveness: if we're close to the next step, take it immediately.
	now := ctx.NowTick()
	interval := uint64(t.stepIntervalTicks())
	if interval == 0 {
		interval = 1
	}
	if now-t.lastStep >= interval/2 {
		t.lastStep = now
		t.step()
	}
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

func (t *Task) stepIntervalTicks() int {
	interval := stepIntervalBaseTicks - t.score/2
	if interval < stepIntervalMinTicks {
		interval = stepIntervalMinTicks
	}
	return interval
}

func (t *Task) initGame() {
	w := t.fb.Width()
	h := t.fb.Height()
	if w <= 0 || h <= 0 {
		t.active = false
		return
	}
	if t.cell <= 0 {
		t.cell = 10
	}

	top := int(t.fontHeight) + 2
	bottom := int(t.fontHeight) + 2
	usableH := h - top - bottom
	if usableH <= 0 {
		t.active = false
		return
	}

	t.gridW = w / t.cell
	t.gridH = usableH / t.cell
	if t.gridW < 8 || t.gridH < 8 {
		t.active = false
		return
	}

	start := point{x: t.gridW / 2, y: t.gridH / 2}
	t.snake = []point{
		start,
		{x: start.x - 1, y: start.y},
		{x: start.x - 2, y: start.y},
	}
	t.headDir = dirRight
	t.nextDir = dirRight
	t.score = 0
	t.alive = true
	t.paused = false
	t.rng = 0x12345678
	t.spawnFood()
}

func (t *Task) step() {
	if !t.alive || len(t.snake) == 0 {
		return
	}

	t.headDir = t.nextDir
	head := t.snake[0]
	next := head
	switch t.headDir {
	case dirUp:
		next.y--
	case dirDown:
		next.y++
	case dirLeft:
		next.x--
	case dirRight:
		next.x++
	}

	if t.gridW > 0 {
		for next.x < 0 {
			next.x += t.gridW
		}
		for next.x >= t.gridW {
			next.x -= t.gridW
		}
	}
	if t.gridH > 0 {
		for next.y < 0 {
			next.y += t.gridH
		}
		for next.y >= t.gridH {
			next.y -= t.gridH
		}
	}

	willEat := next == t.food
	check := t.snake
	if !willEat && len(check) > 1 {
		check = check[:len(check)-1]
	}
	for _, p := range check {
		if p == next {
			t.alive = false
			return
		}
	}

	t.snake = append([]point{next}, t.snake...)
	if willEat {
		t.score++
		t.spawnFood()
		return
	}
	t.snake = t.snake[:len(t.snake)-1]
}

func (t *Task) spawnFood() {
	if t.gridW <= 0 || t.gridH <= 0 {
		return
	}
	for tries := 0; tries < 1024; tries++ {
		t.rng = xorshift32(t.rng)
		x := int(t.rng % uint32(t.gridW))
		t.rng = xorshift32(t.rng)
		y := int(t.rng % uint32(t.gridH))
		p := point{x: x, y: y}
		if !t.occupied(p) {
			t.food = p
			return
		}
	}
	t.food = point{x: 0, y: 0}
}

func (t *Task) occupied(p point) bool {
	for _, s := range t.snake {
		if s == p {
			return true
		}
	}
	return false
}

func xorshift32(x uint32) uint32 {
	if x == 0 {
		x = 0x6d2b79f5
	}
	x ^= x << 13
	x ^= x >> 17
	x ^= x << 5
	return x
}

func (t *Task) render() {
	if !t.active || t.fb == nil || t.fb.Format() != hal.PixelFormatRGB565 {
		return
	}
	buf := t.fb.Buffer()
	if buf == nil {
		return
	}
	w := t.fb.Width()
	h := t.fb.Height()
	if w <= 0 || h <= 0 {
		return
	}

	bg := rgb565From888(0x00, 0x00, 0x00)
	clearRGB565(buf, bg)

	topBar := int(t.fontHeight) + 2
	bottomBar := int(t.fontHeight) + 2
	gridY0 := topBar

	header := fmt.Sprintf("SNAKE score=%d | arrows move | p pause | r restart | q quit", t.score)
	t.drawText(0, 0, header, color.RGBA{R: 0xEE, G: 0xEE, B: 0xEE, A: 0xFF})

	msg := ""
	if !t.alive {
		msg = "GAME OVER (press r)"
	} else if t.paused {
		msg = "PAUSED"
	}
	if msg != "" {
		y := h - bottomBar + 1
		t.drawText(0, y, msg, color.RGBA{R: 0xFF, G: 0xD1, B: 0x4A, A: 0xFF})
	}

	if t.gridW > 0 && t.gridH > 0 && t.cell > 0 {
		foodC := rgb565From888(0xFF, 0x50, 0x50)
		snakeC := rgb565From888(0x50, 0xFF, 0x50)
		headC := rgb565From888(0x50, 0xD1, 0xFF)

		drawCellRGB565(buf, w, t.fb.StrideBytes(), gridY0, t.cell, t.food, foodC)
		for i, p := range t.snake {
			c := snakeC
			if i == 0 {
				c = headC
			}
			drawCellRGB565(buf, w, t.fb.StrideBytes(), gridY0, t.cell, p, c)
		}
	}

	_ = t.fb.Present()
}

func clearRGB565(buf []byte, pixel uint16) {
	lo := byte(pixel)
	hi := byte(pixel >> 8)
	for i := 0; i+1 < len(buf); i += 2 {
		buf[i] = lo
		buf[i+1] = hi
	}
}

func drawCellRGB565(buf []byte, w, stride, y0, cell int, p point, pixel uint16) {
	if cell <= 0 {
		return
	}
	x0 := p.x * cell
	y1 := y0 + p.y*cell
	for y := 0; y < cell; y++ {
		py := y1 + y
		if py < 0 {
			continue
		}
		row := py * stride
		for x := 0; x < cell; x++ {
			px := x0 + x
			if px < 0 || px >= w {
				continue
			}
			off := row + px*2
			if off < 0 || off+1 >= len(buf) {
				continue
			}
			buf[off] = byte(pixel)
			buf[off+1] = byte(pixel >> 8)
		}
	}
}

func rgb565From888(r, g, b uint8) uint16 {
	return uint16((uint16(r>>3)&0x1F)<<11 | (uint16(g>>2)&0x3F)<<5 | (uint16(b>>3) & 0x1F))
}

func (t *Task) drawText(x, y int, s string, c color.RGBA) {
	if t.fb == nil || t.fb.Format() != hal.PixelFormatRGB565 {
		return
	}
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

func (d *fbDisplayer) FillRectangle(x, y, width, height int16, c color.RGBA) error {
	if d.fb == nil || d.fb.Format() != hal.PixelFormatRGB565 {
		return nil
	}
	buf := d.fb.Buffer()
	if buf == nil {
		return nil
	}

	w := d.fb.Width()
	h := d.fb.Height()
	x0 := clampInt(int(x), 0, w)
	y0 := clampInt(int(y), 0, h)
	x1 := clampInt(int(x)+int(width), 0, w)
	y1 := clampInt(int(y)+int(height), 0, h)
	if x0 >= x1 || y0 >= y1 {
		return nil
	}

	pixel := rgb565From888(c.R, c.G, c.B)
	lo := byte(pixel)
	hi := byte(pixel >> 8)
	stride := d.fb.StrideBytes()
	for py := y0; py < y1; py++ {
		row := py * stride
		for px := x0; px < x1; px++ {
			off := row + px*2
			if off < 0 || off+1 >= len(buf) {
				continue
			}
			buf[off] = lo
			buf[off+1] = hi
		}
	}
	return nil
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
