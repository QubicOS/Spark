package fbtest

import (
	"fmt"
	"image/color"
	"math"
	"sort"

	"spark/hal"
	"spark/sparkos/fonts/font6x8cp1251"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"

	"tinygo.org/x/tinyfont"
)

const (
	scoreScale = 1000
)

type benchTest struct {
	name string
	ops  uint64
	run  func(ctx *kernel.Context) uint64
}

type testResult struct {
	name  string
	ticks uint64
	score uint64
}

type Task struct {
	disp hal.Display
	ep   kernel.Capability

	fb hal.Framebuffer

	font       tinyfont.Fonter
	fontWidth  int16
	fontHeight int16

	active  bool
	muxCap  kernel.Capability
	running bool

	w int
	h int

	results    []testResult
	totalScore uint64
	status     string

	inbuf []byte
}

func New(disp hal.Display, ep kernel.Capability) *Task {
	return &Task{disp: disp, ep: ep}
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
	if t.fb == nil || t.fb.Format() != hal.PixelFormatRGB565 {
		return
	}
	if !t.initFont() {
		return
	}

	t.w = t.fb.Width()
	t.h = t.fb.Height()
	if t.w <= 0 || t.h <= 0 {
		return
	}

	for msg := range ch {
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
			appID, _, ok := proto.DecodeAppSelectPayload(msg.Payload())
			if !ok || appID != proto.AppFBTest {
				continue
			}
			if t.active {
				t.runBenchmarks(ctx)
			}

		case proto.MsgTermInput:
			if !t.active {
				continue
			}
			t.handleInput(ctx, msg.Payload())
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
	t.runBenchmarks(ctx)
}

func (t *Task) unload() {
	t.active = false
	t.results = nil
	t.inbuf = nil
}

func (t *Task) requestExit(ctx *kernel.Context) {
	t.active = false
	if !t.muxCap.Valid() {
		return
	}
	_ = ctx.SendToCapRetry(t.muxCap, uint16(proto.MsgAppControl), proto.AppControlPayload(false), kernel.Capability{}, 500)
}

func (t *Task) handleInput(ctx *kernel.Context, b []byte) {
	for _, c := range b {
		switch c {
		case 0x1b, 'q':
			t.requestExit(ctx)
			return
		case 'r':
			t.runBenchmarks(ctx)
			return
		}
	}
}

func (t *Task) runBenchmarks(ctx *kernel.Context) {
	if t.running || t.fb == nil {
		return
	}
	t.running = true
	t.results = t.results[:0]
	t.totalScore = 0
	t.status = "Running..."
	t.render()

	for _, test := range t.benchmarks() {
		ticks := test.run(ctx)
		score := scoreFromOps(test.ops, ticks)
		t.results = append(t.results, testResult{name: test.name, ticks: ticks, score: score})
		t.totalScore += score
		t.render()
	}

	t.status = "Done."
	t.running = false
	t.render()
}

func (t *Task) benchmarks() []benchTest {
	baseOps := uint64(t.w * t.h)
	return []benchTest{
		{name: "Fill", ops: baseOps * 4, run: t.testFill},
		{name: "Text", ops: baseOps / 2, run: t.testText},
		{name: "Pixels", ops: baseOps, run: t.testPixels},
		{name: "Lines", ops: baseOps, run: t.testLines},
		{name: "Fast lines", ops: baseOps, run: t.testFastLines},
		{name: "Rects", ops: baseOps, run: t.testRects},
		{name: "Filled rects", ops: baseOps, run: t.testFilledRects},
		{name: "Circles", ops: baseOps, run: t.testCircles},
		{name: "Filled circles", ops: baseOps, run: t.testFilledCircles},
		{name: "Triangles", ops: baseOps, run: t.testTriangles},
		{name: "Filled triangles", ops: baseOps, run: t.testFilledTriangles},
		{name: "Round rects", ops: baseOps, run: t.testRoundRects},
		{name: "Filled round rects", ops: baseOps, run: t.testFilledRoundRects},
	}
}

func (t *Task) measure(ctx *kernel.Context, fn func()) uint64 {
	start := ctx.NowTick()
	fn()
	end := ctx.NowTick()
	if end <= start {
		end = start + 1
	}
	return end - start
}

func (t *Task) testFill(ctx *kernel.Context) uint64 {
	return t.measure(ctx, func() {
		t.fb.ClearRGB(0xFF, 0xFF, 0xFF)
		_ = t.fb.Present()
		t.fb.ClearRGB(0xFF, 0x00, 0x00)
		_ = t.fb.Present()
		t.fb.ClearRGB(0x00, 0xFF, 0x00)
		_ = t.fb.Present()
		t.fb.ClearRGB(0x00, 0x00, 0xFF)
		_ = t.fb.Present()
		t.fb.ClearRGB(0x00, 0x00, 0x00)
		_ = t.fb.Present()
	})
}

func (t *Task) testText(ctx *kernel.Context) uint64 {
	return t.measure(ctx, func() {
		t.fb.ClearRGB(0x00, 0x00, 0x00)
		t.drawText(10, 10, "Framebuffer benchmark", color.RGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF})
		t.drawText(10, 24, "Shapes, fills, lines, text", color.RGBA{R: 0xA0, G: 0xA0, B: 0xA0, A: 0xFF})
		t.drawText(10, 38, "Press r to rerun, q to exit", color.RGBA{R: 0x80, G: 0xC0, B: 0xFF, A: 0xFF})
		_ = t.fb.Present()
	})
}

func (t *Task) testPixels(ctx *kernel.Context) uint64 {
	buf := t.fb.Buffer()
	if buf == nil {
		return 1
	}
	stride := t.fb.StrideBytes()
	w := t.w
	h := t.h
	return t.measure(ctx, func() {
		clearRGB565(buf, rgb565From888(0x00, 0x00, 0x00))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				pixel := rgb565From888(uint8(x), uint8(y), uint8(x*y))
				setPixelRGB565(buf, stride, x, y, pixel)
			}
		}
		_ = t.fb.Present()
	})
}

func (t *Task) testLines(ctx *kernel.Context) uint64 {
	buf := t.fb.Buffer()
	if buf == nil {
		return 1
	}
	stride := t.fb.StrideBytes()
	w := t.w
	h := t.h
	c := rgb565From888(0x00, 0x80, 0xFF)
	return t.measure(ctx, func() {
		clearRGB565(buf, rgb565From888(0x00, 0x00, 0x00))
		for x := 0; x < w; x += 6 {
			drawLineRGB565(buf, stride, 0, 0, x, h-1, c)
		}
		for y := 0; y < h; y += 6 {
			drawLineRGB565(buf, stride, 0, 0, w-1, y, c)
		}
		_ = t.fb.Present()
	})
}

func (t *Task) testFastLines(ctx *kernel.Context) uint64 {
	buf := t.fb.Buffer()
	if buf == nil {
		return 1
	}
	stride := t.fb.StrideBytes()
	w := t.w
	h := t.h
	return t.measure(ctx, func() {
		clearRGB565(buf, rgb565From888(0x00, 0x00, 0x00))
		red := rgb565From888(0xFF, 0x30, 0x30)
		blue := rgb565From888(0x30, 0x80, 0xFF)
		for y := 0; y < h; y += 5 {
			fillRectRGB565(buf, stride, 0, y, w, 1, red)
		}
		for x := 0; x < w; x += 5 {
			fillRectRGB565(buf, stride, x, 0, 1, h, blue)
		}
		_ = t.fb.Present()
	})
}

func (t *Task) testRects(ctx *kernel.Context) uint64 {
	buf := t.fb.Buffer()
	if buf == nil {
		return 1
	}
	stride := t.fb.StrideBytes()
	w := t.w
	h := t.h
	c := rgb565From888(0x20, 0xFF, 0x60)
	return t.measure(ctx, func() {
		clearRGB565(buf, rgb565From888(0x00, 0x00, 0x00))
		n := minInt(w, h)
		cx := w / 2
		cy := h / 2
		for i := 2; i < n; i += 6 {
			half := i / 2
			drawRectOutlineRGB565(buf, stride, cx-half, cy-half, i, i, c)
		}
		_ = t.fb.Present()
	})
}

func (t *Task) testFilledRects(ctx *kernel.Context) uint64 {
	buf := t.fb.Buffer()
	if buf == nil {
		return 1
	}
	stride := t.fb.StrideBytes()
	w := t.w
	h := t.h
	return t.measure(ctx, func() {
		clearRGB565(buf, rgb565From888(0x00, 0x00, 0x00))
		n := minInt(w, h)
		cx := w/2 - 1
		cy := h/2 - 1
		for i := n; i > 0; i -= 6 {
			half := i / 2
			fillRectRGB565(buf, stride, cx-half, cy-half, i, i, rgb565From888(0xFF, 0xE0, 0x40))
		}
		_ = t.fb.Present()
	})
}

func (t *Task) testCircles(ctx *kernel.Context) uint64 {
	buf := t.fb.Buffer()
	if buf == nil {
		return 1
	}
	stride := t.fb.StrideBytes()
	w := t.w
	h := t.h
	radius := 10
	return t.measure(ctx, func() {
		clearRGB565(buf, rgb565From888(0x00, 0x00, 0x00))
		c := rgb565From888(0xFF, 0xFF, 0xFF)
		for x := 0; x < w+radius; x += radius * 2 {
			for y := 0; y < h+radius; y += radius * 2 {
				drawCircleRGB565(buf, stride, x, y, radius, c)
			}
		}
		_ = t.fb.Present()
	})
}

func (t *Task) testFilledCircles(ctx *kernel.Context) uint64 {
	buf := t.fb.Buffer()
	if buf == nil {
		return 1
	}
	stride := t.fb.StrideBytes()
	w := t.w
	h := t.h
	radius := 10
	return t.measure(ctx, func() {
		clearRGB565(buf, rgb565From888(0x00, 0x00, 0x00))
		c := rgb565From888(0xFF, 0x00, 0xFF)
		for x := radius; x < w; x += radius * 2 {
			for y := radius; y < h; y += radius * 2 {
				fillCircleRGB565(buf, stride, x, y, radius, c)
			}
		}
		_ = t.fb.Present()
	})
}

func (t *Task) testTriangles(ctx *kernel.Context) uint64 {
	buf := t.fb.Buffer()
	if buf == nil {
		return 1
	}
	stride := t.fb.StrideBytes()
	w := t.w
	h := t.h
	return t.measure(ctx, func() {
		clearRGB565(buf, rgb565From888(0x00, 0x00, 0x00))
		cx := w/2 - 1
		cy := h/2 - 1
		limit := minInt(cx, cy)
		for i := 0; i < limit; i += 6 {
			drawTriangleRGB565(
				buf,
				stride,
				cx,
				cy-i,
				cx-i,
				cy+i,
				cx+i,
				cy+i,
				rgb565From888(uint8(i), 0x00, uint8(255-i)),
			)
		}
		_ = t.fb.Present()
	})
}

func (t *Task) testFilledTriangles(ctx *kernel.Context) uint64 {
	buf := t.fb.Buffer()
	if buf == nil {
		return 1
	}
	stride := t.fb.StrideBytes()
	w := t.w
	h := t.h
	return t.measure(ctx, func() {
		clearRGB565(buf, rgb565From888(0x00, 0x00, 0x00))
		cx := w/2 - 1
		cy := h/2 - 1
		limit := minInt(cx, cy)
		for i := limit; i > 10; i -= 6 {
			fillTriangleRGB565(
				buf,
				stride,
				cx,
				cy-i,
				cx-i,
				cy+i,
				cx+i,
				cy+i,
				rgb565From888(0x00, uint8(i), 0xA0),
			)
		}
		_ = t.fb.Present()
	})
}

func (t *Task) testRoundRects(ctx *kernel.Context) uint64 {
	buf := t.fb.Buffer()
	if buf == nil {
		return 1
	}
	stride := t.fb.StrideBytes()
	w := t.w
	h := t.h
	return t.measure(ctx, func() {
		clearRGB565(buf, rgb565From888(0x00, 0x00, 0x00))
		n := minInt(w, h)
		cx := w/2 - 1
		cy := h/2 - 1
		for i := 0; i < n; i += 6 {
			half := i / 2
			r := maxInt(2, i/8)
			drawRoundRectRGB565(buf, stride, cx-half, cy-half, i, i, r, rgb565From888(uint8(i), 0x20, 0x20))
		}
		_ = t.fb.Present()
	})
}

func (t *Task) testFilledRoundRects(ctx *kernel.Context) uint64 {
	buf := t.fb.Buffer()
	if buf == nil {
		return 1
	}
	stride := t.fb.StrideBytes()
	w := t.w
	h := t.h
	return t.measure(ctx, func() {
		clearRGB565(buf, rgb565From888(0x00, 0x00, 0x00))
		n := minInt(w, h)
		cx := w/2 - 1
		cy := h/2 - 1
		for i := n; i > 20; i -= 6 {
			half := i / 2
			r := maxInt(2, i/8)
			fillRoundRectRGB565(buf, stride, cx-half, cy-half, i, i, r, rgb565From888(0x00, uint8(i), 0x00))
		}
		_ = t.fb.Present()
	})
}

func scoreFromOps(ops, ticks uint64) uint64 {
	if ticks == 0 {
		ticks = 1
	}
	return (ops * scoreScale) / ticks
}

func (t *Task) render() {
	if !t.active || t.fb == nil {
		return
	}
	buf := t.fb.Buffer()
	if buf == nil {
		return
	}

	clearRGB565(buf, rgb565From888(0x08, 0x0B, 0x10))
	pad := 8
	title := "FBTEST"
	t.drawText(pad, pad, title, color.RGBA{R: 0xEE, G: 0xEE, B: 0xEE, A: 0xFF})

	sub := "r rerun  q quit"
	t.drawText(pad+int(t.fontWidth)*len(title)+8, pad+1, sub, color.RGBA{R: 0x88, G: 0xA6, B: 0xD6, A: 0xFF})

	y := pad + int(t.fontHeight) + 6
	if t.status != "" {
		t.drawText(pad, y, t.status, color.RGBA{R: 0xB0, G: 0xB0, B: 0xB0, A: 0xFF})
		y += int(t.fontHeight) + 4
	}

	header := "Test                 ticks     score"
	t.drawText(pad, y, header, color.RGBA{R: 0xAA, G: 0xAA, B: 0xAA, A: 0xFF})
	y += int(t.fontHeight) + 2

	for _, res := range t.results {
		line := fmt.Sprintf("%-18s %8d %9d", res.name, res.ticks, res.score)
		t.drawText(pad, y, line, color.RGBA{R: 0xE0, G: 0xE0, B: 0xE0, A: 0xFF})
		y += int(t.fontHeight) + 2
	}

	if len(t.results) > 0 {
		y += 4
		totalLine := fmt.Sprintf("Total score: %d", t.totalScore)
		t.drawText(pad, y, totalLine, color.RGBA{R: 0x4A, G: 0xD1, B: 0xFF, A: 0xFF})
	}

	_ = t.fb.Present()
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

func setPixelRGB565(buf []byte, stride, x, y int, pixel uint16) {
	off := y*stride + x*2
	if off < 0 || off+1 >= len(buf) {
		return
	}
	buf[off] = byte(pixel)
	buf[off+1] = byte(pixel >> 8)
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

func drawLineRGB565(buf []byte, stride, x0, y0, x1, y1 int, pixel uint16) {
	dx := absInt(x1 - x0)
	sx := -1
	if x0 < x1 {
		sx = 1
	}
	dy := -absInt(y1 - y0)
	sy := -1
	if y0 < y1 {
		sy = 1
	}
	err := dx + dy
	for {
		setPixelRGB565(buf, stride, x0, y0, pixel)
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * err
		if e2 >= dy {
			if x0 == x1 {
				return
			}
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			if y0 == y1 {
				return
			}
			err += dx
			y0 += sy
		}
	}
}

func drawCircleRGB565(buf []byte, stride, cx, cy, r int, pixel uint16) {
	x := r
	y := 0
	err := 0
	for x >= y {
		setCirclePoints(buf, stride, cx, cy, x, y, pixel)
		y++
		if err <= 0 {
			err += 2*y + 1
		}
		if err > 0 {
			x--
			err -= 2*x + 1
		}
	}
}

func fillCircleRGB565(buf []byte, stride, cx, cy, r int, pixel uint16) {
	for y := -r; y <= r; y++ {
		dx := int(math.Sqrt(float64(r*r - y*y)))
		fillRectRGB565(buf, stride, cx-dx, cy+y, dx*2+1, 1, pixel)
	}
}

func setCirclePoints(buf []byte, stride, cx, cy, x, y int, pixel uint16) {
	setPixelRGB565(buf, stride, cx+x, cy+y, pixel)
	setPixelRGB565(buf, stride, cx+y, cy+x, pixel)
	setPixelRGB565(buf, stride, cx-x, cy+y, pixel)
	setPixelRGB565(buf, stride, cx-y, cy+x, pixel)
	setPixelRGB565(buf, stride, cx-x, cy-y, pixel)
	setPixelRGB565(buf, stride, cx-y, cy-x, pixel)
	setPixelRGB565(buf, stride, cx+x, cy-y, pixel)
	setPixelRGB565(buf, stride, cx+y, cy-x, pixel)
}

func drawTriangleRGB565(buf []byte, stride, x0, y0, x1, y1, x2, y2 int, pixel uint16) {
	drawLineRGB565(buf, stride, x0, y0, x1, y1, pixel)
	drawLineRGB565(buf, stride, x1, y1, x2, y2, pixel)
	drawLineRGB565(buf, stride, x2, y2, x0, y0, pixel)
}

func fillTriangleRGB565(buf []byte, stride, x0, y0, x1, y1, x2, y2 int, pixel uint16) {
	type pt struct {
		x int
		y int
	}
	pts := []pt{{x: x0, y: y0}, {x: x1, y: y1}, {x: x2, y: y2}}
	sort.Slice(pts, func(i, j int) bool { return pts[i].y < pts[j].y })
	top := pts[0]
	mid := pts[1]
	bot := pts[2]
	if bot.y == top.y {
		return
	}
	drawSpan := func(y, xStart, xEnd int) {
		if xStart > xEnd {
			xStart, xEnd = xEnd, xStart
		}
		fillRectRGB565(buf, stride, xStart, y, xEnd-xStart+1, 1, pixel)
	}
	for y := top.y; y <= bot.y; y++ {
		var xa, xb float64
		if y < mid.y {
			xa = edgeX(top, mid, y)
			xb = edgeX(top, bot, y)
		} else {
			xa = edgeX(mid, bot, y)
			xb = edgeX(top, bot, y)
		}
		drawSpan(y, int(math.Round(xa)), int(math.Round(xb)))
	}
}

func edgeX(a, b struct{ x, y int }, y int) float64 {
	if b.y == a.y {
		return float64(a.x)
	}
	t := float64(y-a.y) / float64(b.y-a.y)
	return float64(a.x) + t*float64(b.x-a.x)
}

func drawRoundRectRGB565(buf []byte, stride, x0, y0, w, h, r int, pixel uint16) {
	if w <= 0 || h <= 0 {
		return
	}
	if r < 1 {
		drawRectOutlineRGB565(buf, stride, x0, y0, w, h, pixel)
		return
	}
	drawRectOutlineRGB565(buf, stride, x0+r, y0, w-2*r, h, pixel)
	drawRectOutlineRGB565(buf, stride, x0, y0+r, w, h-2*r, pixel)
	drawCircleQuadrants(buf, stride, x0+r, y0+r, r, pixel, true, true, false, false)
	drawCircleQuadrants(buf, stride, x0+w-r-1, y0+r, r, pixel, false, true, true, false)
	drawCircleQuadrants(buf, stride, x0+r, y0+h-r-1, r, pixel, true, false, false, true)
	drawCircleQuadrants(buf, stride, x0+w-r-1, y0+h-r-1, r, pixel, false, false, true, true)
}

func fillRoundRectRGB565(buf []byte, stride, x0, y0, w, h, r int, pixel uint16) {
	if w <= 0 || h <= 0 {
		return
	}
	if r < 1 {
		fillRectRGB565(buf, stride, x0, y0, w, h, pixel)
		return
	}
	fillRectRGB565(buf, stride, x0+r, y0, w-2*r, h, pixel)
	fillRectRGB565(buf, stride, x0, y0+r, w, h-2*r, pixel)
	fillCircleQuadrants(buf, stride, x0+r, y0+r, r, pixel, true, true, false, false)
	fillCircleQuadrants(buf, stride, x0+w-r-1, y0+r, r, pixel, false, true, true, false)
	fillCircleQuadrants(buf, stride, x0+r, y0+h-r-1, r, pixel, true, false, false, true)
	fillCircleQuadrants(buf, stride, x0+w-r-1, y0+h-r-1, r, pixel, false, false, true, true)
}

func drawCircleQuadrants(
	buf []byte,
	stride int,
	cx int,
	cy int,
	r int,
	pixel uint16,
	q1 bool,
	q2 bool,
	q3 bool,
	q4 bool,
) {
	x := r
	y := 0
	err := 0
	for x >= y {
		if q1 {
			setPixelRGB565(buf, stride, cx-x, cy-y, pixel)
			setPixelRGB565(buf, stride, cx-y, cy-x, pixel)
		}
		if q2 {
			setPixelRGB565(buf, stride, cx+x, cy-y, pixel)
			setPixelRGB565(buf, stride, cx+y, cy-x, pixel)
		}
		if q3 {
			setPixelRGB565(buf, stride, cx+x, cy+y, pixel)
			setPixelRGB565(buf, stride, cx+y, cy+x, pixel)
		}
		if q4 {
			setPixelRGB565(buf, stride, cx-x, cy+y, pixel)
			setPixelRGB565(buf, stride, cx-y, cy+x, pixel)
		}
		y++
		if err <= 0 {
			err += 2*y + 1
		}
		if err > 0 {
			x--
			err -= 2*x + 1
		}
	}
}

func fillCircleQuadrants(
	buf []byte,
	stride int,
	cx int,
	cy int,
	r int,
	pixel uint16,
	q1 bool,
	q2 bool,
	q3 bool,
	q4 bool,
) {
	for y := -r; y <= r; y++ {
		dx := int(math.Sqrt(float64(r*r - y*y)))
		if y <= 0 {
			if q1 {
				fillRectRGB565(buf, stride, cx-dx, cy+y, dx, 1, pixel)
			}
			if q2 {
				fillRectRGB565(buf, stride, cx, cy+y, dx+1, 1, pixel)
			}
		}
		if y >= 0 {
			if q4 {
				fillRectRGB565(buf, stride, cx-dx, cy+y, dx, 1, pixel)
			}
			if q3 {
				fillRectRGB565(buf, stride, cx, cy+y, dx+1, 1, pixel)
			}
		}
	}
}

func rgb565From888(r, g, b uint8) uint16 {
	return uint16(r&0xF8)<<8 | uint16(g&0xFC)<<3 | uint16(b>>3)
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
