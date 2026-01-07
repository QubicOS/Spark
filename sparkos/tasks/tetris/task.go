package tetris

import (
	"fmt"
	"image/color"

	"spark/hal"
	"spark/sparkos/fonts/font6x8cp1251"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"

	"tinygo.org/x/tinyfont"
)

type point struct {
	x int
	y int
}

type pieceID uint8

const (
	pieceI pieceID = iota
	pieceO
	pieceT
	pieceS
	pieceZ
	pieceJ
	pieceL
)

type Task struct {
	disp hal.Display
	ep   kernel.Capability

	fb hal.Framebuffer

	font       tinyfont.Fonter
	fontWidth  int16
	fontHeight int16

	active bool
	muxCap kernel.Capability

	w int
	h int

	cell int

	boardW int
	boardH int

	board []uint8 // 0 empty, else color index 1..7

	curID  pieceID
	curRot int
	curPos point

	nextID pieceID

	rng uint32

	score int
	lines int
	level int

	paused   bool
	gameOver bool

	nowTick uint64

	clearActive  bool
	clearStarted uint64
	clearRows    [4]int
	clearCount   int

	lastFall uint64

	inbuf []byte
}

const (
	boardWidth  = 10
	boardHeight = 20

	fallBaseTicks = 70
	fallMinTicks  = 10

	clearFlashTicks = 18
)

func New(disp hal.Display, ep kernel.Capability) *Task {
	return &Task{disp: disp, ep: ep, cell: 12}
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
				if !ok || appID != proto.AppTetris {
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
			if !t.active || t.paused || t.gameOver {
				continue
			}
			t.nowTick = now
			if t.clearActive {
				if now-t.clearStarted >= clearFlashTicks {
					t.finishClear()
				}
				t.render()
				continue
			}
			interval := uint64(t.fallIntervalTicks())
			if interval == 0 {
				interval = 1
			}
			if now-t.lastFall < interval {
				continue
			}
			t.lastFall = now
			if !t.tryMove(0, 1) {
				t.lockOrClear(now)
			}
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
	t.lastFall = ctx.NowTick()
	t.render()
}

func (t *Task) unload() {
	t.active = false
	t.board = nil
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
	switch k.kind {
	case keyEsc:
		t.requestExit(ctx)
	case keyLeft:
		if !t.paused && !t.gameOver && !t.clearActive {
			_ = t.tryMove(-1, 0)
		}
	case keyRight:
		if !t.paused && !t.gameOver && !t.clearActive {
			_ = t.tryMove(1, 0)
		}
	case keyDown:
		if !t.paused && !t.gameOver && !t.clearActive {
			if t.tryMove(0, 1) {
				t.score += 1
			}
		}
	case keyUp:
		if !t.paused && !t.gameOver && !t.clearActive {
			t.rotate(1)
		}
	case keyRune:
		switch k.r {
		case 'q':
			t.requestExit(ctx)
		case 'p', ' ':
			if !t.gameOver {
				t.paused = !t.paused
			}
		case 'r':
			t.initGame()
			t.lastFall = ctx.NowTick()
		case 'z':
			if !t.paused && !t.gameOver && !t.clearActive {
				t.rotate(-1)
			}
		case 'x':
			if !t.paused && !t.gameOver && !t.clearActive {
				t.rotate(1)
			}
		case 'c':
			if !t.paused && !t.gameOver && !t.clearActive {
				t.hardDrop()
			}
		}
	}
}

func (t *Task) initGame() {
	if t.fb == nil {
		t.active = false
		return
	}
	t.w = t.fb.Width()
	t.h = t.fb.Height()
	if t.w <= 0 || t.h <= 0 {
		t.active = false
		return
	}
	if t.cell <= 0 {
		t.cell = 12
	}

	t.boardW = boardWidth
	t.boardH = boardHeight
	t.board = make([]uint8, t.boardW*t.boardH)

	t.score = 0
	t.lines = 0
	t.level = 1
	t.paused = false
	t.gameOver = false
	t.rng = 0x9e3779b9
	t.clearActive = false
	t.clearStarted = 0
	t.clearCount = 0

	t.nextID = t.nextPiece()
	t.spawnPiece()
}

func (t *Task) fallIntervalTicks() int {
	interval := fallBaseTicks - (t.level-1)*6
	if interval < fallMinTicks {
		interval = fallMinTicks
	}
	return interval
}

func (t *Task) nextPiece() pieceID {
	t.rng = xorshift32(t.rng)
	return pieceID(t.rng % 7)
}

func (t *Task) spawnPiece() {
	t.curID = t.nextID
	t.curRot = 0
	t.curPos = point{x: t.boardW/2 - 2, y: 0}
	t.nextID = t.nextPiece()
	if t.collides(t.curPos.x, t.curPos.y, t.curRot) {
		t.gameOver = true
	}
}

func (t *Task) rotate(delta int) {
	nr := t.curRot + delta
	for nr < 0 {
		nr += 4
	}
	nr %= 4

	// Basic wall kicks.
	for _, dx := range []int{0, -1, 1, -2, 2} {
		if !t.collides(t.curPos.x+dx, t.curPos.y, nr) {
			t.curPos.x += dx
			t.curRot = nr
			return
		}
	}
}

func (t *Task) tryMove(dx, dy int) bool {
	if !t.collides(t.curPos.x+dx, t.curPos.y+dy, t.curRot) {
		t.curPos.x += dx
		t.curPos.y += dy
		return true
	}
	return false
}

func (t *Task) hardDrop() {
	for !t.collides(t.curPos.x, t.curPos.y+1, t.curRot) {
		t.curPos.y++
		t.score += 2
	}
	t.lockOrClear(t.nowTick)
}

func (t *Task) lockOrClear(now uint64) {
	blocks := pieceBlocks(t.curID, t.curRot)
	for _, b := range blocks {
		x := t.curPos.x + b.x
		y := t.curPos.y + b.y
		if x < 0 || x >= t.boardW || y < 0 || y >= t.boardH {
			continue
		}
		t.board[y*t.boardW+x] = uint8(t.curID) + 1
	}

	t.clearCount = t.findFullRows(t.clearRows[:])
	if t.clearCount > 0 {
		t.clearActive = true
		t.clearStarted = now
		return
	}
	t.spawnPiece()
}

func (t *Task) findFullRows(dst []int) int {
	n := 0
	for y := t.boardH - 1; y >= 0; y-- {
		full := true
		rowOff := y * t.boardW
		for x := 0; x < t.boardW; x++ {
			if t.board[rowOff+x] == 0 {
				full = false
				break
			}
		}
		if !full {
			continue
		}
		if n < len(dst) {
			dst[n] = y
		}
		n++
	}
	if n > len(dst) {
		n = len(dst)
	}
	return n
}

func (t *Task) finishClear() {
	if t.clearCount <= 0 {
		t.clearActive = false
		t.clearStarted = 0
		return
	}

	skip := [boardHeight]bool{}
	for i := 0; i < t.clearCount; i++ {
		y := t.clearRows[i]
		if y >= 0 && y < t.boardH {
			skip[y] = true
		}
	}

	writeY := t.boardH - 1
	for y := t.boardH - 1; y >= 0; y-- {
		if skip[y] {
			continue
		}
		if writeY != y {
			copy(t.board[writeY*t.boardW:(writeY+1)*t.boardW], t.board[y*t.boardW:(y+1)*t.boardW])
		}
		writeY--
	}
	for y := writeY; y >= 0; y-- {
		row := t.board[y*t.boardW : (y+1)*t.boardW]
		for x := range row {
			row[x] = 0
		}
	}

	cleared := t.clearCount
	t.lines += cleared
	t.score += 100 * cleared * cleared * t.level
	t.level = 1 + t.lines/10

	t.clearActive = false
	t.clearStarted = 0
	t.clearCount = 0
	t.spawnPiece()
}

func (t *Task) isClearRow(y int) bool {
	if !t.clearActive || t.clearCount <= 0 {
		return false
	}
	for i := 0; i < t.clearCount; i++ {
		if t.clearRows[i] == y {
			return true
		}
	}
	return false
}

func (t *Task) collides(px, py, rot int) bool {
	blocks := pieceBlocks(t.curID, rot)
	for _, b := range blocks {
		x := px + b.x
		y := py + b.y
		if x < 0 || x >= t.boardW || y < 0 || y >= t.boardH {
			return true
		}
		if t.board[y*t.boardW+x] != 0 {
			return true
		}
	}
	return false
}

func pieceBlocks(id pieceID, rot int) [4]point {
	// Shapes are defined in a 4x4 grid as offsets.
	// Rotation is applied around the 4x4 grid origin with simple transforms.
	base := [4]point{}
	switch id {
	case pieceI:
		base = [4]point{{0, 1}, {1, 1}, {2, 1}, {3, 1}}
	case pieceO:
		base = [4]point{{1, 0}, {2, 0}, {1, 1}, {2, 1}}
	case pieceT:
		base = [4]point{{1, 0}, {0, 1}, {1, 1}, {2, 1}}
	case pieceS:
		base = [4]point{{1, 0}, {2, 0}, {0, 1}, {1, 1}}
	case pieceZ:
		base = [4]point{{0, 0}, {1, 0}, {1, 1}, {2, 1}}
	case pieceJ:
		base = [4]point{{0, 0}, {0, 1}, {1, 1}, {2, 1}}
	case pieceL:
		base = [4]point{{2, 0}, {0, 1}, {1, 1}, {2, 1}}
	}

	rot %= 4
	if rot < 0 {
		rot += 4
	}

	out := base
	for i := 0; i < 4; i++ {
		x := base[i].x
		y := base[i].y
		switch rot {
		case 0:
			out[i] = point{x: x, y: y}
		case 1:
			out[i] = point{x: 3 - y, y: x}
		case 2:
			out[i] = point{x: 3 - x, y: 3 - y}
		case 3:
			out[i] = point{x: y, y: 3 - x}
		}
	}
	return out
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

	bg := rgb565From888(0x00, 0x00, 0x00)
	clearRGB565(buf, bg)

	t.drawText(0, 0, "TETRIS", color.RGBA{R: 0xEE, G: 0xEE, B: 0xEE, A: 0xFF})
	stats := fmt.Sprintf("score=%d lines=%d lvl=%d", t.score, t.lines, t.level)
	t.drawText(0, int(t.fontHeight)+1, stats, color.RGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xFF})

	msg := ""
	if t.gameOver {
		msg = "GAME OVER (press r)"
	} else if t.paused {
		msg = "PAUSED"
	}
	if msg != "" {
		y := t.h - int(t.fontHeight) - 1
		t.drawText(0, y, msg, color.RGBA{R: 0xFF, G: 0xD1, B: 0x4A, A: 0xFF})
	}

	top := int(t.fontHeight)*2 + 4
	playW := t.boardW * t.cell
	playH := t.boardH * t.cell
	left := 8
	if t.w > playW+16 {
		left = (t.w - playW) / 2
	}
	if left < 4 {
		left = 4
	}

	border := rgb565From888(0x55, 0x55, 0x55)
	drawRectOutlineRGB565(buf, t.fb.StrideBytes(), left-1, top-1, playW+2, playH+2, border)

	// Draw settled blocks.
	flashClear := t.clearActive && ((t.nowTick/8)%2 == 0)
	for y := 0; y < t.boardH; y++ {
		for x := 0; x < t.boardW; x++ {
			if flashClear && t.isClearRow(y) {
				drawCellRGB565(buf, t.fb.StrideBytes(), left+x*t.cell, top+y*t.cell, t.cell, rgb565From888(0xFF, 0xFF, 0xFF))
				continue
			}
			v := t.board[y*t.boardW+x]
			if v == 0 {
				continue
			}
			drawCellRGB565(buf, t.fb.StrideBytes(), left+x*t.cell, top+y*t.cell, t.cell, palette565(v))
		}
	}

	// Draw ghost piece.
	if !t.gameOver && !t.clearActive {
		ghost := t.curPos
		for !t.collides(ghost.x, ghost.y+1, t.curRot) {
			ghost.y++
		}
		if ghost.y != t.curPos.y {
			blocks := pieceBlocks(t.curID, t.curRot)
			for _, b := range blocks {
				x := ghost.x + b.x
				y := ghost.y + b.y
				if x < 0 || x >= t.boardW || y < 0 || y >= t.boardH {
					continue
				}
				drawCellRGB565(buf, t.fb.StrideBytes(), left+x*t.cell, top+y*t.cell, t.cell, rgb565From888(0x22, 0x22, 0x22))
			}
		}
	}

	// Draw current piece.
	if !t.gameOver && !t.clearActive {
		blocks := pieceBlocks(t.curID, t.curRot)
		for _, b := range blocks {
			x := t.curPos.x + b.x
			y := t.curPos.y + b.y
			if x < 0 || x >= t.boardW || y < 0 || y >= t.boardH {
				continue
			}
			drawCellRGB565(buf, t.fb.StrideBytes(), left+x*t.cell, top+y*t.cell, t.cell, palette565(uint8(t.curID)+1))
		}
	}

	// Side panel: next piece preview.
	panelX := left + playW + t.cell
	panelY := top
	if panelX+6*t.cell < t.w {
		t.drawText(panelX, panelY, "NEXT", color.RGBA{R: 0xEE, G: 0xEE, B: 0xEE, A: 0xFF})
		boxY := panelY + int(t.fontHeight) + 2
		boxW := 4*t.cell + 2
		boxH := 4*t.cell + 2
		drawRectOutlineRGB565(buf, t.fb.StrideBytes(), panelX-1, boxY-1, boxW, boxH, border)

		blocks := pieceBlocks(t.nextID, 0)
		for _, b := range blocks {
			x := b.x
			y := b.y
			// Center a 4x4 piece box; pieceBlocks already in 4x4 coordinates.
			drawCellRGB565(buf, t.fb.StrideBytes(), panelX+x*t.cell, boxY+y*t.cell, t.cell, palette565(uint8(t.nextID)+1))
		}
	}

	_ = t.fb.Present()
}

func palette565(v uint8) uint16 {
	switch v {
	case 1:
		return rgb565From888(0x4A, 0xD1, 0xFF) // I
	case 2:
		return rgb565From888(0xFF, 0xD1, 0x4A) // O
	case 3:
		return rgb565From888(0xFF, 0x7F, 0xFF) // T
	case 4:
		return rgb565From888(0x7F, 0xFF, 0x7F) // S
	case 5:
		return rgb565From888(0xFF, 0x50, 0x50) // Z
	case 6:
		return rgb565From888(0x50, 0x50, 0xFF) // J
	case 7:
		return rgb565From888(0xFF, 0xA0, 0x50) // L
	default:
		return rgb565From888(0x55, 0x55, 0x55)
	}
}

func clearRGB565(buf []byte, pixel uint16) {
	lo := byte(pixel)
	hi := byte(pixel >> 8)
	for i := 0; i+1 < len(buf); i += 2 {
		buf[i] = lo
		buf[i+1] = hi
	}
}

func drawCellRGB565(buf []byte, stride, x0, y0, cell int, pixel uint16) {
	if cell <= 0 {
		return
	}
	lo := byte(pixel)
	hi := byte(pixel >> 8)
	for y := 0; y < cell; y++ {
		row := (y0+y)*stride + x0*2
		for x := 0; x < cell; x++ {
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
