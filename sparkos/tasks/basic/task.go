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

type tab uint8

const (
	tabCode tab = iota
	tabIO
	tabVars
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

	tab tab

	output []string

	input      []rune
	inputCur   int
	statusLine string
	inbuf      []byte

	codeSel int
	codeTop int

	codeEdit    bool
	codeEditBuf []rune
	codeEditCur int

	varTop int

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
		tab:        tabIO,
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
	t.inbuf = nil
	t.vm = nil
	t.vfs = nil
	t.prog = program{}
	t.awaitInput = false
	t.awaitVar = varRef{}
	t.tab = tabIO
	t.codeSel = 0
	t.codeTop = 0
	t.codeEdit = false
	t.codeEditBuf = nil
	t.codeEditCur = 0
	t.varTop = 0
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
	case keyF1:
		t.tab = tabCode
		return
	case keyF2:
		t.tab = tabIO
		return
	case keyF3:
		t.tab = tabVars
		return
	case keyEsc:
		t.requestExit(ctx)
		return
	}

	switch t.tab {
	case tabCode:
		t.handleCodeKey(ctx, k)
	case tabVars:
		t.handleVarsKey(ctx, k)
	default:
		t.handleIOKey(ctx, k)
	}
}

func (t *Task) handleIOKey(ctx *kernel.Context, k key) {
	// IO tab is runtime-only: accept input only for INPUT prompts.
	if !t.awaitInput {
		switch k.kind {
		case keyRune:
			switch k.r {
			case 'r', 'R':
				t.onLine(ctx, "RUN")
				return
			case 's', 'S':
				t.tab = tabVars
				return
			}
		}
		return
	}

	switch k.kind {
	case keyEnter:
		line := strings.TrimSpace(string(t.input))
		t.clearInput()
		if line != "" {
			t.onLine(ctx, line)
		}
	case keyBackspace:
		if t.inputCur > 0 {
			t.input = append(t.input[:t.inputCur-1], t.input[t.inputCur:]...)
			t.inputCur--
		}
	case keyRune:
		if len(t.input) >= maxInputRunes {
			return
		}
		if t.inputCur == len(t.input) {
			t.input = append(t.input, k.r)
			t.inputCur = len(t.input)
			return
		}
		t.input = append(t.input[:t.inputCur], append([]rune{k.r}, t.input[t.inputCur:]...)...)
		t.inputCur++
	}
}

func (t *Task) handleCodeKey(ctx *kernel.Context, k key) {
	if t.codeEdit {
		t.handleCodeEditKey(ctx, k)
		return
	}

	switch k.kind {
	case keyUp:
		if t.codeSel > 0 {
			t.codeSel--
		}
		if t.codeSel < t.codeTop {
			t.codeTop = t.codeSel
		}
	case keyDown:
		if t.codeSel < len(t.prog.lines)-1 {
			t.codeSel++
		}
		if t.codeSel >= t.codeTop+(t.rows-2) {
			t.codeTop = t.codeSel - (t.rows - 2) + 1
		}
	case keyHome:
		t.codeSel = 0
		t.codeTop = 0
	case keyEnd:
		if len(t.prog.lines) > 0 {
			t.codeSel = len(t.prog.lines) - 1
			if t.codeSel >= t.rows-2 {
				t.codeTop = t.codeSel - (t.rows - 2) + 1
			}
		}
	case keyEnter:
		if t.codeSel >= 0 && t.codeSel < len(t.prog.lines) {
			t.codeEdit = true
			t.codeEditBuf = []rune(t.prog.lines[t.codeSel].String())
			t.codeEditCur = len(t.codeEditBuf)
			t.statusLine = "Edit line. Enter commit | Esc cancel | Del delete line."
		}
	case keyDelete:
		if t.codeSel >= 0 && t.codeSel < len(t.prog.lines) {
			no := t.prog.lines[t.codeSel].no
			t.prog.deleteLine(no)
			if t.codeSel >= len(t.prog.lines) && t.codeSel > 0 {
				t.codeSel--
			}
			if t.codeTop > t.codeSel {
				t.codeTop = t.codeSel
			}
		}
	case keyRune:
		switch k.r {
		case 'n', 'N':
			t.codeEdit = true
			t.codeEditBuf = nil
			t.codeEditCur = 0
			t.statusLine = "New line: type '<num> <stmt>' then Enter."
		case 'r', 'R':
			t.onLine(ctx, "RUN")
			t.tab = tabIO
		case 'l', 'L':
			t.statusLine = "F1 code | Enter edit | n new | Del delete | r run | Esc exit"
		}
	}
}

func (t *Task) handleCodeEditKey(ctx *kernel.Context, k key) {
	switch k.kind {
	case keyEsc:
		t.codeEdit = false
		t.codeEditBuf = nil
		t.codeEditCur = 0
		t.statusLine = "Canceled."
		return
	case keyEnter:
		line := strings.TrimSpace(string(t.codeEditBuf))
		t.codeEdit = false
		t.codeEditBuf = nil
		t.codeEditCur = 0
		if line == "" {
			t.statusLine = "OK"
			return
		}
		t.onLine(ctx, line)
		// Keep selection stable if possible.
		if t.codeSel >= len(t.prog.lines) && len(t.prog.lines) > 0 {
			t.codeSel = len(t.prog.lines) - 1
		}
		return
	case keyBackspace:
		if t.codeEditCur > 0 {
			t.codeEditBuf = append(t.codeEditBuf[:t.codeEditCur-1], t.codeEditBuf[t.codeEditCur:]...)
			t.codeEditCur--
		}
		return
	case keyDelete:
		if t.codeEditCur < len(t.codeEditBuf) {
			t.codeEditBuf = append(t.codeEditBuf[:t.codeEditCur], t.codeEditBuf[t.codeEditCur+1:]...)
		}
		return
	case keyLeft:
		if t.codeEditCur > 0 {
			t.codeEditCur--
		}
		return
	case keyRight:
		if t.codeEditCur < len(t.codeEditBuf) {
			t.codeEditCur++
		}
		return
	case keyHome:
		t.codeEditCur = 0
		return
	case keyEnd:
		t.codeEditCur = len(t.codeEditBuf)
		return
	case keyRune:
		if len(t.codeEditBuf) >= 512 {
			return
		}
		if t.codeEditCur == len(t.codeEditBuf) {
			t.codeEditBuf = append(t.codeEditBuf, k.r)
			t.codeEditCur = len(t.codeEditBuf)
			return
		}
		t.codeEditBuf = append(t.codeEditBuf[:t.codeEditCur], append([]rune{k.r}, t.codeEditBuf[t.codeEditCur:]...)...)
		t.codeEditCur++
		return
	default:
		return
	}
}

func (t *Task) handleVarsKey(ctx *kernel.Context, k key) {
	switch k.kind {
	case keyUp:
		if t.varTop > 0 {
			t.varTop--
		}
	case keyDown:
		t.varTop++
	case keyPageUp:
		t.varTop -= 8
	case keyPageDown:
		t.varTop += 8
	case keyRune:
		switch k.r {
		case 'r', 'R':
			t.onLine(ctx, "RUN")
		case 's', 'S':
			if t.vm != nil && t.vm.running {
				t.runSteps(ctx, 1)
				return
			}
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
			t.runSteps(ctx, 1)
		case 'x', 'X':
			if t.vm != nil {
				t.vm.running = false
			}
		}
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

	t.renderHeader()

	switch t.tab {
	case tabCode:
		t.renderCode()
	case tabVars:
		t.renderVars()
	default:
		t.renderIO()
	}

	if t.statusLine != "" {
		tinyfont.WriteLine(
			t.d,
			t.font,
			0,
			int16(t.rows-1)*t.fontHeight+t.fontOffset,
			t.statusLine,
			color.RGBA{R: 0x80, G: 0x80, B: 0x80, A: 0xff},
		)
	}

	_ = t.fb.Present()
}

func (t *Task) renderHeader() {
	head := "BASIC | F1 code  F2 io  F3 vars"
	switch t.tab {
	case tabCode:
		head = "BASIC | F1 code* F2 io  F3 vars"
	case tabIO:
		head = "BASIC | F1 code  F2 io* F3 vars"
	case tabVars:
		head = "BASIC | F1 code  F2 io  F3 vars*"
	}
	tinyfont.WriteLine(t.d, t.font, 0, t.fontOffset, head, color.RGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xff})
}

func (t *Task) renderIO() {
	visible := t.rows - 3 // header + input + status
	if visible < 0 {
		visible = 0
	}
	row := 0
	start := 0
	if len(t.output) > visible {
		start = len(t.output) - visible
	}
	for i := start; i < len(t.output) && row < visible; i++ {
		tinyfont.WriteLine(
			t.d,
			t.font,
			0,
			int16(row+1)*t.fontHeight+t.fontOffset,
			t.output[i],
			color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff},
		)
		row++
	}

	prompt := ">"
	if t.awaitInput {
		prompt = "?"
	}
	if !t.awaitInput {
		tinyfont.WriteLine(
			t.d,
			t.font,
			0,
			int16(t.rows-2)*t.fontHeight+t.fontOffset,
			"[not running] press r to RUN or F1 to edit",
			color.RGBA{R: 0x80, G: 0x80, B: 0x80, A: 0xff},
		)
		return
	}

	in := prompt + string(t.input)
	tinyfont.WriteLine(
		t.d,
		t.font,
		0,
		int16(t.rows-2)*t.fontHeight+t.fontOffset,
		in,
		color.RGBA{R: 0xff, G: 0xff, B: 0, A: 0xff},
	)
}

func (t *Task) renderCode() {
	if t.codeSel >= len(t.prog.lines) {
		t.codeSel = len(t.prog.lines) - 1
	}
	if t.codeSel < 0 {
		t.codeSel = 0
	}
	if t.codeTop < 0 {
		t.codeTop = 0
	}
	if t.codeTop > t.codeSel {
		t.codeTop = t.codeSel
	}

	visible := t.rows - 2 // header + status
	for row := 0; row < visible; row++ {
		i := t.codeTop + row
		y := int16(row+1)*t.fontHeight + t.fontOffset
		if i < 0 || i >= len(t.prog.lines) {
			continue
		}
		c := color.RGBA{R: 0xee, G: 0xee, B: 0xee, A: 0xff}
		prefix := "  "
		if i == t.codeSel {
			c = color.RGBA{R: 0xff, G: 0xff, B: 0x4a, A: 0xff}
			prefix = "> "
		}
		line := prefix + t.prog.lines[i].String()
		if t.codeEdit && i == t.codeSel {
			line = prefix + insertCaret(string(t.codeEditBuf), t.codeEditCur)
		}
		tinyfont.WriteLine(t.d, t.font, 0, y, line, c)
	}
}

func insertCaret(s string, cur int) string {
	r := []rune(s)
	if cur < 0 {
		cur = 0
	}
	if cur > len(r) {
		cur = len(r)
	}
	r = append(r[:cur], append([]rune{'|'}, r[cur:]...)...)
	return string(r)
}

func (t *Task) renderVars() {
	var lines []string
	if t.vm != nil && t.vm.running && t.vm.prog != nil && t.vm.pc >= 0 && t.vm.pc < len(t.vm.prog.lines) {
		lines = append(lines, fmt.Sprintf("RUNNING line %d", t.vm.prog.lines[t.vm.pc].no))
	} else {
		lines = append(lines, "STOPPED")
	}
	if t.vm != nil {
		lines = append(lines, fmt.Sprintf("stack: gosub=%d for=%d", len(t.vm.callStack), len(t.vm.forStack)))
		open := 0
		for i := range t.vm.files {
			if t.vm.files[i].inUse {
				open++
			}
		}
		lines = append(lines, fmt.Sprintf("files: open=%d/%d", open, len(t.vm.files)))
	}
	lines = append(lines, "ints:")
	if t.vm != nil {
		for i := 0; i < 26; i++ {
			v := t.vm.intVars[i]
			if v == 0 {
				continue
			}
			lines = append(lines, fmt.Sprintf("  %c=%d", 'A'+byte(i), v))
		}
	}
	lines = append(lines, "strings:")
	if t.vm != nil {
		for i := 0; i < 26; i++ {
			v := t.vm.strVars[i]
			if v == "" {
				continue
			}
			lines = append(lines, fmt.Sprintf("  %c$=%q", 'A'+byte(i), v))
		}
	}
	lines = append(lines, "arrays:")
	if t.vm != nil {
		for i := 0; i < 26; i++ {
			if t.vm.arrVars[i] == nil {
				continue
			}
			lines = append(lines, fmt.Sprintf("  %c(%d)", 'A'+byte(i), len(t.vm.arrVars[i])-1))
		}
	}
	lines = append(lines, "debug: r run | s step | x stop")

	visible := t.rows - 2 // header + status
	if t.varTop < 0 {
		t.varTop = 0
	}
	maxTop := 0
	if len(lines) > visible {
		maxTop = len(lines) - visible
	}
	if t.varTop > maxTop {
		t.varTop = maxTop
	}

	for row := 0; row < visible; row++ {
		i := t.varTop + row
		if i < 0 || i >= len(lines) {
			continue
		}
		tinyfont.WriteLine(t.d, t.font, 0, int16(row+1)*t.fontHeight+t.fontOffset, lines[i], color.RGBA{R: 0xee, G: 0xee, B: 0xee, A: 0xff})
	}
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
