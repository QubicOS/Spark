package basic

import (
	"errors"
	"fmt"
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
	hint       string
	inbuf      []byte

	codeEd codeEditor

	showHelp bool
	helpTop  int

	varTop int

	prog program
	vm   *vm

	awaitInput bool
	awaitVar   varRef

	nowTick uint64
	blinkOn bool
}

func New(disp hal.Display, ep kernel.Capability, vfsCap kernel.Capability) *Task {
	return &Task{
		disp:       disp,
		ep:         ep,
		vfsCap:     vfsCap,
		tab:        tabIO,
		statusLine: "TinyBASIC: F1 code | F2 io | F3 vars | Esc exit | H help.",
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
		case now := <-tickCh:
			if !t.active {
				continue
			}
			t.nowTick = now
			blink := (now/350)%2 == 0
			if blink != t.blinkOn {
				t.blinkOn = blink
				if t.tab == tabCode {
					t.render()
				}
			}

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
	t.codeEd = codeEditor{}
	t.varTop = 0
	t.showHelp = false
	t.helpTop = 0
	t.hint = ""
	t.statusLine = ""
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
		if len(t.codeEd.lines) == 0 {
			t.codeEd.loadFromProgram(&t.prog)
		}
		t.showHelp = false
		return
	case keyF2:
		_ = t.syncEditorToProgram()
		t.tab = tabIO
		t.showHelp = false
		return
	case keyF3:
		_ = t.syncEditorToProgram()
		t.tab = tabVars
		t.showHelp = false
		return
	case keyEsc:
		// In the code editor, Esc is a normal key (no hotkeys except F1/F2/F3).
		if t.tab == tabCode {
			t.codeEd.handleKey(k, t.rows-2, t.cols)
			t.hint = t.codeEd.hint
			return
		}
		if t.showHelp {
			t.showHelp = false
			return
		}
		t.requestExit(ctx)
		return
	}

	// No non-F hotkeys in code editor.
	if t.tab == tabCode {
		t.codeEd.handleKey(k, t.rows-2, t.cols)
		t.hint = t.codeEd.hint
		return
	}

	if k.kind == keyRune && (k.r == 'H' || k.r == 'h') {
		t.showHelp = !t.showHelp
		if t.showHelp {
			t.helpTop = 0
		}
		return
	}
	if t.showHelp {
		t.handleHelpKey(k)
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

func (t *Task) handleHelpKey(k key) {
	switch k.kind {
	case keyEsc, keyEnter:
		t.showHelp = false
	case keyUp:
		if t.helpTop > 0 {
			t.helpTop--
		}
	case keyDown:
		t.helpTop++
	case keyHome:
		t.helpTop = 0
	case keyEnd:
		t.helpTop = 1 << 30
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
	_ = ctx
	t.codeEd.handleKey(k, t.rows-2, t.cols)
	t.hint = t.codeEd.hint
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
			if err := t.syncEditorToProgram(); err != nil {
				t.printf("? %v", err)
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
		if err := t.syncEditorToProgram(); err != nil {
			t.printf("? %v", err)
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
		t.runSteps(ctx, 1_000_000)
		return
	case "LIST":
		for _, ln := range t.prog.lines {
			t.println(ln.String())
		}
		return
	case "NEW":
		t.prog = program{}
		t.codeEd = codeEditor{}
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

func (t *Task) syncEditorToProgram() error {
	if len(t.codeEd.lines) == 0 {
		t.codeEd.loadFromProgram(&t.prog)
		return nil
	}

	var p program
	lines := t.codeEd.toLines()
	for _, raw := range lines {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		no, rest, ok := splitLeadingLineNumber(raw)
		if !ok {
			return errors.New("all lines must start with a line number")
		}
		if strings.TrimSpace(rest) == "" {
			continue
		}
		p.upsertLine(no, rest)
	}
	t.prog = p
	return nil
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
	t.fb.ClearRGB(colorBG.R, colorBG.G, colorBG.B)

	t.renderHeader()

	switch t.tab {
	case tabCode:
		t.renderCode()
	case tabVars:
		t.renderVars()
	default:
		t.renderIO()
	}

	t.renderStatus()
	if t.showHelp {
		t.renderHelp()
	}

	_ = t.fb.Present()
}

func (t *Task) renderHeader() {
	w := int16(t.cols) * t.fontWidth
	_ = t.d.FillRectangle(0, 0, w, t.fontHeight, colorHeaderBG)

	head := "BASIC | F1 code  F2 io  F3 vars"
	switch t.tab {
	case tabCode:
		head = "BASIC | F1 code* F2 io  F3 vars"
	case tabIO:
		head = "BASIC | F1 code  F2 io* F3 vars"
	case tabVars:
		head = "BASIC | F1 code  F2 io  F3 vars*"
	}
	tinyfont.WriteLine(t.d, t.font, 0, t.fontOffset, clipRunes(head, t.cols), colorDim)
}

func (t *Task) renderStatus() {
	w := int16(t.cols) * t.fontWidth
	y := int16(t.rows-1) * t.fontHeight
	_ = t.d.FillRectangle(0, y, w, t.fontHeight, colorStatusBG)

	s := t.statusLine
	if t.hint != "" && s != "" {
		s = t.hint + " | " + s
	} else if t.hint != "" {
		s = t.hint
	}
	tinyfont.WriteLine(t.d, t.font, 0, y+t.fontOffset, clipRunes(s, t.cols), colorDim)
}

func (t *Task) renderHelp() {
	lines := t.helpLines()
	if len(lines) == 0 {
		return
	}
	boxW := int16(t.cols-4) * t.fontWidth
	if boxW < 0 {
		return
	}
	boxH := int16(t.rows-4) * t.fontHeight
	if boxH < 0 {
		return
	}
	x := int16(2) * t.fontWidth
	y := int16(2) * t.fontHeight
	_ = t.d.FillRectangle(x, y, boxW, boxH, colorHeaderBG)

	innerCols := t.cols - 4
	innerRows := t.rows - 4
	if innerCols <= 0 || innerRows <= 0 {
		return
	}

	maxTop := 0
	if len(lines) > innerRows {
		maxTop = len(lines) - innerRows
	}
	if t.helpTop < 0 {
		t.helpTop = 0
	}
	if t.helpTop > maxTop {
		t.helpTop = maxTop
	}

	start := t.helpTop
	for row := 0; row < innerRows; row++ {
		i := start + row
		if i >= len(lines) {
			break
		}
		tinyfont.WriteLine(
			t.d,
			t.font,
			x,
			y+int16(row+1)*t.fontHeight+t.fontOffset,
			clipRunes(lines[i], innerCols),
			colorFG,
		)
	}
	tinyfont.WriteLine(t.d, t.font, x, y+t.fontOffset, clipRunes("HELP (Esc to close)", innerCols), colorAccent)
}

func (t *Task) helpLines() []string {
	switch t.tab {
	case tabCode:
		return []string{
			"F1/F2/F3: tabs",
			"Arrows: move cursor",
			"Enter: newline",
			"Backspace/Delete: delete",
			"Tab: accept completion",
			"Note: lines must start with a number to RUN.",
		}
	case tabVars:
		return []string{
			"Up/Down/PgUp/PgDn: scroll",
			"r: run",
			"s: step",
			"x: stop",
		}
	default:
		return []string{
			"INPUT mode: type value + Enter",
			"r: RUN",
			"Esc: exit app",
		}
	}
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
		tinyfont.WriteLine(t.d, t.font, 0, int16(row+1)*t.fontHeight+t.fontOffset, clipRunes(t.output[i], t.cols), colorFG)
		row++
	}

	prompt := ">"
	if t.awaitInput {
		prompt = "?"
	}
	if !t.awaitInput {
		tinyfont.WriteLine(t.d, t.font, 0, int16(t.rows-2)*t.fontHeight+t.fontOffset, clipRunes("[not running] press r to RUN or F1 to edit", t.cols), colorDim)
		t.hint = "io: r run | H help"
		return
	}

	in := prompt + string(t.input)
	tinyfont.WriteLine(t.d, t.font, 0, int16(t.rows-2)*t.fontHeight+t.fontOffset, clipRunes(in, t.cols), colorInputFG)
	t.hint = "io: enter submit | H help"
}

func (t *Task) renderCode() {
	visible := t.rows - 2 // header + status
	t.codeEd.ensureNonEmpty()
	t.codeEd.ensureVisible(visible, t.cols)

	rows := t.codeEd.viewRowCount(visible)
	for row := 0; row < rows; row++ {
		idx := t.codeEd.top + row
		y0 := int16(row+1) * t.fontHeight
		y := y0 + t.fontOffset

		if idx == t.codeEd.curRow {
			_ = t.d.FillRectangle(0, y0, int16(t.cols)*t.fontWidth, t.fontHeight, colorHeaderBG)
		}
		drawBasicLine(t.d, t.font, t.fontWidth, 0, y, t.codeEd.lineRunes(idx), t.codeEd.left, t.cols)
	}

	// Cursor and ghost.
	if t.blinkOn && t.codeEd.curRow >= t.codeEd.top && t.codeEd.curRow < t.codeEd.top+visible {
		row := t.codeEd.curRow - t.codeEd.top
		cx := t.codeEd.curCol - t.codeEd.left
		if cx >= 0 && cx < t.cols {
			x := int16(cx) * t.fontWidth
			y := int16(row+1) * t.fontHeight
			_ = t.d.FillRectangle(x, y, t.fontWidth, t.fontHeight, colorAccent)
			r := t.codeEd.cursorRune()
			tinyfont.WriteLine(t.d, t.font, x, y+t.fontOffset, string(r), colorBG)
		}
	}
	if t.codeEd.ghost != "" && t.codeEd.curRow >= t.codeEd.top && t.codeEd.curRow < t.codeEd.top+visible {
		row := t.codeEd.curRow - t.codeEd.top
		cx := t.codeEd.curCol - t.codeEd.left
		if cx >= 0 && cx < t.cols {
			x := int16(cx) * t.fontWidth
			y := int16(row+1)*t.fontHeight + t.fontOffset
			tinyfont.WriteLine(t.d, t.font, x, y, clipRunes(t.codeEd.ghost, t.cols-cx), colorDim)
		}
	}

	t.hint = t.codeEd.hint
	if t.hint == "" {
		t.hint = "code: arrows move | Tab complete | F2 io | F3 vars"
	}
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
		tinyfont.WriteLine(t.d, t.font, 0, int16(row+1)*t.fontHeight+t.fontOffset, clipRunes(lines[i], t.cols), colorFG)
	}
	t.hint = "vars: Up/Down scroll | r run | s step | x stop | H help"
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
