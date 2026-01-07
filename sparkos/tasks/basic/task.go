package basic

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

const (
	maxOutputLines = 64
	maxInputRunes  = 160

	defaultMaxFiles = 8
)

// Task provides a minimal Tiny BASIC-like interpreter.
type Task struct {
	disp   hal.Display
	ep     kernel.Capability
	vfsCap kernel.Capability

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

	vfs *vfsclient.Client

	output []string

	input      []rune
	inputCur   int
	statusLine string

	prog program
	vm   *vm

	awaitInput bool
	awaitVar   varRef
}

func New(disp hal.Display, ep kernel.Capability, vfsCap kernel.Capability) *Task {
	return &Task{
		disp:       disp,
		ep:         ep,
		vfsCap:     vfsCap,
		statusLine: "TinyBASIC: RUN/LIST/NEW, Ctrl+G to exit.",
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
			t.setActive(active)
			if t.active {
				t.render()
			}

		case proto.MsgAppSelect:
			appID, arg, ok := proto.DecodeAppSelectPayload(msg.Data[:msg.Len])
			if !ok || appID != proto.AppBasic {
				continue
			}
			if arg != "" {
				t.statusLine = arg
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
	}
}

func (t *Task) initFont() bool {
	t.font = font6x8cp1251.Font
	t.fontHeight = 8
	t.fontOffset = 7
	_, outboxWidth := tinyfont.LineWidth(t.font, "0")
	t.fontWidth = int16(outboxWidth)
	return t.fontWidth > 0 && t.fontHeight > 0
}

func (t *Task) setActive(active bool) {
	if active == t.active {
		return
	}
	t.active = active
	if !t.active {
		return
	}
	if t.vm == nil {
		t.vm = newVM(defaultMaxFiles)
	}
}

func (t *Task) unload() {
	t.output = nil
	t.input = nil
	t.vm = nil
	t.vfs = nil
	t.prog = program{}
	t.awaitInput = false
	t.awaitVar = varRef{}
}

func (t *Task) vfsClient() *vfsclient.Client {
	if t.vfs == nil {
		t.vfs = vfsclient.New(t.vfsCap)
	}
	return t.vfs
}

func (t *Task) println(s string) {
	if len(t.output) >= maxOutputLines {
		copy(t.output, t.output[1:])
		t.output[len(t.output)-1] = s
		return
	}
	t.output = append(t.output, s)
}

func (t *Task) printf(format string, args ...any) {
	t.println(fmt.Sprintf(format, args...))
}

func (t *Task) clearInput() {
	t.input = t.input[:0]
	t.inputCur = 0
}

func (t *Task) handleInput(ctx *kernel.Context, b []byte) {
	for len(b) > 0 {
		if b[0] == 0x1b {
			if len(b) == 1 {
				t.requestExit(ctx)
				return
			}
			if b[1] != '[' {
				t.requestExit(ctx)
				return
			}
			n := consumeCSI(b)
			if n == 0 {
				return
			}
			b = b[n:]
			continue
		}

		r, n := decodeUTF8Rune(b)
		if n == 0 {
			return
		}
		b = b[n:]

		switch r {
		case '\r', '\n':
			line := strings.TrimSpace(string(t.input))
			t.clearInput()
			if line != "" {
				t.onLine(ctx, line)
			}
			continue
		case '\b', 0x7f:
			if t.inputCur > 0 {
				t.input = append(t.input[:t.inputCur-1], t.input[t.inputCur:]...)
				t.inputCur--
			}
			continue
		}

		if len(t.input) >= maxInputRunes {
			continue
		}
		if t.inputCur == len(t.input) {
			t.input = append(t.input, r)
			t.inputCur = len(t.input)
			continue
		}
		t.input = append(t.input[:t.inputCur], append([]rune{r}, t.input[t.inputCur:]...)...)
		t.inputCur++
	}
}

func (t *Task) onLine(ctx *kernel.Context, line string) {
	if t.awaitInput {
		t.awaitInput = false
		if err := t.vm.setFromInput(t.awaitVar, line); err != nil {
			t.printf("? %v", err)
		}
		t.awaitVar = varRef{}
		if t.vm.running {
			t.runSteps(ctx, 1_000_000)
		}
		return
	}

	lineNo, rest, hasLineNo := splitLeadingLineNumber(line)
	if hasLineNo {
		if strings.TrimSpace(rest) == "" {
			t.prog.deleteLine(lineNo)
		} else {
			t.prog.upsertLine(lineNo, rest)
		}
		t.statusLine = "OK"
		return
	}

	cmd := strings.ToUpper(firstWord(line))
	switch cmd {
	case "RUN":
		t.vm.reset()
		t.vm.prog = &t.prog
		t.vm.vfs = t.vfsClient()
		t.vm.ctx = ctx
		t.vm.fb = t.fb
		if err := t.vm.start(); err != nil {
			t.printf("? %v", err)
			t.vm.running = false
			return
		}
		t.runSteps(ctx, 1_000_000)
		return
	case "LIST":
		for _, ln := range t.prog.lines {
			t.println(ln.String())
		}
		return
	case "NEW":
		t.prog = program{}
		t.vm.reset()
		t.statusLine = "OK"
		return
	default:
		t.vm.reset()
		t.vm.prog = &t.prog
		t.vm.vfs = t.vfsClient()
		t.vm.ctx = ctx
		t.vm.fb = t.fb
		res, err := t.vm.execImmediate(line)
		if err != nil {
			t.printf("? %v", err)
			return
		}
		for _, out := range res.output {
			t.println(out)
		}
		return
	}
}

func (t *Task) runSteps(ctx *kernel.Context, limit int) {
	if t.vm == nil || !t.vm.running {
		return
	}
	for i := 0; i < limit && t.vm.running; i++ {
		step, err := t.vm.step()
		if err != nil {
			t.printf("? %v", err)
			t.vm.running = false
			return
		}
		if step.awaitInput {
			t.awaitInput = true
			t.awaitVar = step.awaitVar
			t.statusLine = "? "
			return
		}
		for _, out := range step.output {
			t.println(out)
		}
		if step.halt {
			t.vm.running = false
			t.statusLine = "OK"
			return
		}
	}
}

func (t *Task) render() {
	if t.fb == nil {
		return
	}
	t.fb.ClearRGB(0, 0, 0)

	row := 0
	start := 0
	if len(t.output) > t.viewRows {
		start = len(t.output) - t.viewRows
	}
	for i := start; i < len(t.output) && row < t.viewRows; i++ {
		tinyfont.WriteLine(t.d, t.font, 0, int16(row)*t.fontHeight+t.fontOffset, t.output[i], color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff})
		row++
	}

	prompt := ">"
	if t.awaitInput {
		prompt = "?"
	}
	in := prompt + string(t.input)
	tinyfont.WriteLine(t.d, t.font, 0, int16(t.viewRows)*t.fontHeight+t.fontOffset, in, color.RGBA{R: 0xff, G: 0xff, B: 0, A: 0xff})

	if t.statusLine != "" {
		tinyfont.WriteLine(t.d, t.font, 0, int16(t.viewRows+1)*t.fontHeight+t.fontOffset, t.statusLine, color.RGBA{R: 0x80, G: 0x80, B: 0x80, A: 0xff})
	}

	_ = t.fb.Present()
}

func (t *Task) requestExit(ctx *kernel.Context) {
	if !t.muxCap.Valid() {
		t.active = false
		return
	}
	for {
		res := ctx.SendToCapResult(t.muxCap, uint16(proto.MsgAppControl), proto.AppControlPayload(false), kernel.Capability{})
		switch res {
		case kernel.SendOK:
			t.active = false
			return
		case kernel.SendErrQueueFull:
			ctx.BlockOnTick()
		default:
			t.active = false
			return
		}
	}
}

func consumeCSI(b []byte) int {
	if len(b) < 2 || b[0] != 0x1b || b[1] != '[' {
		return 0
	}
	// Consume until a final byte in the range 0x40..0x7e.
	for i := 2; i < len(b); i++ {
		if b[i] >= 0x40 && b[i] <= 0x7e {
			return i + 1
		}
	}
	return 0
}

func splitLeadingLineNumber(line string) (lineNo int, rest string, ok bool) {
	i := 0
	for i < len(line) && line[i] == ' ' {
		i++
	}
	start := i
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	if i == start {
		return 0, "", false
	}
	n, err := strconv.Atoi(line[start:i])
	if err != nil || n < 0 || n > 65535 {
		return 0, "", false
	}
	return n, strings.TrimSpace(line[i:]), true
}

func firstWord(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' || s[i] == '\t' {
			return s[:i]
		}
	}
	return s
}
