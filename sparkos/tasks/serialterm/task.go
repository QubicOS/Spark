package serialterm

import (
	"fmt"
	"image/color"

	"spark/hal"
	serialclient "spark/sparkos/client/serial"
	"spark/sparkos/fonts/font6x8cp1251"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"

	"tinygo.org/x/tinyfont"
)

const (
	exitCtrlQ  = 0x11
	clearCtrlR = 0x12
)

type Task struct {
	disp      hal.Display
	serialCap kernel.Capability
	ep        kernel.Capability

	fb hal.Framebuffer

	font       tinyfont.Fonter
	fontWidth  int16
	fontHeight int16

	active bool
	muxCap kernel.Capability

	w int
	h int

	lines []string
	cur   []rune

	lastCR bool

	rxTotal uint64
	txTotal uint64
	status  string

	inbuf []byte
}

func New(disp hal.Display, serialCap, ep kernel.Capability) *Task {
	return &Task{disp: disp, serialCap: serialCap, ep: ep}
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

	rxEP := ctx.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	if !rxEP.Valid() {
		return
	}
	rxRecv := rxEP.Restrict(kernel.RightRecv)
	rxSend := rxEP.Restrict(kernel.RightSend)
	if !rxRecv.Valid() || !rxSend.Valid() {
		return
	}
	_ = serialclient.Subscribe(ctx, t.serialCap, rxSend)

	rxCh, ok := ctx.RecvChan(rxRecv)
	if !ok {
		return
	}

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}
			if t.handleAppMsg(ctx, msg) {
				return
			}

		case msg, ok := <-rxCh:
			if !ok {
				return
			}
			if proto.Kind(msg.Kind) != proto.MsgSerialData {
				continue
			}
			t.handleSerialData(msg.Payload())
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

func (t *Task) handleAppMsg(ctx *kernel.Context, msg kernel.Message) bool {
	switch proto.Kind(msg.Kind) {
	case proto.MsgAppShutdown:
		t.unload()
		return true

	case proto.MsgAppControl:
		if msg.Cap.Valid() {
			t.muxCap = msg.Cap
		}
		active, ok := proto.DecodeAppControlPayload(msg.Payload())
		if !ok {
			return false
		}
		t.setActive(active)

	case proto.MsgAppSelect:
		appID, _, ok := proto.DecodeAppSelectPayload(msg.Payload())
		if !ok || appID != proto.AppSerialTerm {
			return false
		}
		if t.active {
			t.status = "Ready."
			t.render()
		}

	case proto.MsgTermInput:
		if !t.active {
			return false
		}
		t.handleInput(ctx, msg.Payload())
	}
	return false
}

func (t *Task) setActive(active bool) {
	if active == t.active {
		return
	}
	t.active = active
	if !t.active {
		return
	}
	if len(t.lines) == 0 && len(t.cur) == 0 {
		t.status = "Ready."
	}
	t.render()
}

func (t *Task) unload() {
	t.active = false
	t.lines = nil
	t.cur = nil
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
	var sendBuf []byte
	for _, c := range b {
		switch c {
		case exitCtrlQ:
			t.requestExit(ctx)
			return
		case clearCtrlR:
			t.lines = t.lines[:0]
			t.cur = t.cur[:0]
			t.status = "Cleared."
			t.render()
			continue
		}
		t.txTotal++
		sendBuf = append(sendBuf, c)
	}
	for len(sendBuf) > 0 {
		chunk := sendBuf
		if len(chunk) > kernel.MaxMessageBytes {
			chunk = chunk[:kernel.MaxMessageBytes]
		}
		_ = serialclient.Write(ctx, t.serialCap, chunk)
		sendBuf = sendBuf[len(chunk):]
	}
}

func (t *Task) handleSerialData(b []byte) {
	for _, c := range b {
		t.rxTotal++
		switch c {
		case '\r':
			t.lastCR = true
			t.pushLine()
		case '\n':
			if t.lastCR {
				t.lastCR = false
				continue
			}
			t.lastCR = false
			t.pushLine()
		case '\t':
			t.lastCR = false
			t.appendRunes([]rune{' ', ' ', ' ', ' '})
		case 0x08:
			t.lastCR = false
			t.backspace()
		default:
			if c < 0x20 {
				continue
			}
			t.lastCR = false
			if c > 0x7e {
				t.appendRunes([]rune{'.'})
				continue
			}
			t.appendRunes([]rune{rune(c)})
		}
	}
	if t.active {
		t.render()
	}
}

func (t *Task) appendRunes(rs []rune) {
	maxChars := t.maxChars()
	for _, r := range rs {
		if len(t.cur) >= maxChars {
			t.pushLine()
		}
		t.cur = append(t.cur, r)
	}
}

func (t *Task) backspace() {
	if len(t.cur) == 0 {
		if len(t.lines) == 0 {
			return
		}
		last := t.lines[len(t.lines)-1]
		t.lines = t.lines[:len(t.lines)-1]
		t.cur = []rune(last)
		return
	}
	t.cur = t.cur[:len(t.cur)-1]
}

func (t *Task) pushLine() {
	t.lines = append(t.lines, string(t.cur))
	t.cur = t.cur[:0]
	maxLines := t.maxLines()
	if len(t.lines) > maxLines {
		t.lines = t.lines[len(t.lines)-maxLines:]
	}
}

func (t *Task) maxChars() int {
	w := t.w - 16
	if w <= 0 || t.fontWidth <= 0 {
		return 1
	}
	return w / int(t.fontWidth)
}

func (t *Task) maxLines() int {
	h := t.h - 32
	if h <= 0 || t.fontHeight <= 0 {
		return 1
	}
	return h / int(t.fontHeight+2)
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
	title := "SERIAL TERMINAL"
	t.drawText(pad, pad, title, color.RGBA{R: 0xEE, G: 0xEE, B: 0xEE, A: 0xFF})

	help := "Ctrl+Q exit  Ctrl+R clear"
	t.drawText(pad, pad+int(t.fontHeight)+2, help, color.RGBA{R: 0x88, G: 0xA6, B: 0xD6, A: 0xFF})

	stats := fmt.Sprintf("RX %d  TX %d", t.rxTotal, t.txTotal)
	t.drawText(pad, pad+int(t.fontHeight)*2+4, stats, color.RGBA{R: 0xAA, G: 0xAA, B: 0xAA, A: 0xFF})

	y := pad + int(t.fontHeight)*3 + 10
	if t.status != "" {
		t.drawText(pad, y, t.status, color.RGBA{R: 0xB0, G: 0xB0, B: 0xB0, A: 0xFF})
		y += int(t.fontHeight) + 4
	}

	maxLines := t.maxLines()
	start := 0
	if len(t.lines) > maxLines {
		start = len(t.lines) - maxLines
	}
	for i := start; i < len(t.lines); i++ {
		t.drawText(pad, y, t.lines[i], color.RGBA{R: 0xE0, G: 0xE0, B: 0xE0, A: 0xFF})
		y += int(t.fontHeight) + 2
	}
	if len(t.cur) > 0 {
		t.drawText(pad, y, string(t.cur), color.RGBA{R: 0xE0, G: 0xE0, B: 0xE0, A: 0xFF})
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

func rgb565From888(r, g, b uint8) uint16 {
	return uint16(r&0xF8)<<8 | uint16(g&0xFC)<<3 | uint16(b>>3)
}
