package vector

import (
	"fmt"
	"math"
	"strings"

	"spark/hal"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"

	"tinygo.org/x/tinyfont"
)

type mode uint8

const (
	modeCalc mode = iota
	modeGraph
)

// Task implements a framebuffer-based math calculator with graphing.
type Task struct {
	disp hal.Display
	ep   kernel.Capability

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

	e *env

	mode mode

	lines []string

	input  []rune
	cursor int

	history []string
	histPos int

	inbuf []byte

	showHelp bool
	helpTop  int

	message string

	graphExpr string
	graph     node

	xMin float64
	xMax float64
	yMin float64
	yMax float64
}

func New(disp hal.Display, ep kernel.Capability) *Task {
	return &Task{
		disp: disp,
		ep:   ep,
		e:    newEnv(),
		mode: modeCalc,
		xMin: -10,
		xMax: 10,
		yMin: -10,
		yMax: 10,
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

	t.appendLine("Vector: calculator + graph.")
	t.appendLine("Type `sin(x)` then press Enter; press `g` to plot.")

	for msg := range ch {
		switch proto.Kind(msg.Kind) {
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
			appID, arg, ok := proto.DecodeAppSelectPayload(msg.Data[:msg.Len])
			if !ok || appID != proto.AppVector {
				continue
			}
			if arg != "" {
				t.setInput(arg)
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

func (t *Task) setActive(ctx *kernel.Context, active bool) {
	if active == t.active {
		return
	}
	t.active = active
	if !t.active {
		return
	}
	t.setMessage("H help | Enter eval | g graph | q quit")
	t.render()
}

func (t *Task) setMessage(msg string) {
	t.message = msg
}

func (t *Task) requestExit(ctx *kernel.Context) {
	if t.fb != nil {
		t.fb.ClearRGB(0, 0, 0)
		_ = t.fb.Present()
	}
	t.active = false
	t.showHelp = false

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

	switch t.mode {
	case modeGraph:
		t.handleGraphKey(ctx, k)
		return
	default:
	}

	switch k.kind {
	case keyEsc:
		t.requestExit(ctx)
	case keyEnter:
		t.submit(ctx)
	case keyBackspace:
		t.backspace()
	case keyDelete:
		t.deleteForward()
	case keyLeft:
		if t.cursor > 0 {
			t.cursor--
		}
	case keyRight:
		if t.cursor < len(t.input) {
			t.cursor++
		}
	case keyHome:
		t.cursor = 0
	case keyEnd:
		t.cursor = len(t.input)
	case keyUp:
		t.histUp()
	case keyDown:
		t.histDown()
	case keyRune:
		switch k.r {
		case 'q':
			t.requestExit(ctx)
		case 'g':
			t.toggleGraph(ctx)
		default:
			if k.r >= 0x20 && k.r != 0x7f {
				t.insertRune(k.r)
			}
		}
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

func (t *Task) insertRune(r rune) {
	if len(t.input) >= 256 {
		return
	}
	t.input = append(t.input, 0)
	copy(t.input[t.cursor+1:], t.input[t.cursor:])
	t.input[t.cursor] = r
	t.cursor++
}

func (t *Task) backspace() {
	if t.cursor <= 0 || len(t.input) == 0 {
		return
	}
	copy(t.input[t.cursor-1:], t.input[t.cursor:])
	t.input = t.input[:len(t.input)-1]
	t.cursor--
}

func (t *Task) deleteForward() {
	if t.cursor < 0 || t.cursor >= len(t.input) {
		return
	}
	copy(t.input[t.cursor:], t.input[t.cursor+1:])
	t.input = t.input[:len(t.input)-1]
}

func (t *Task) setInput(s string) {
	t.input = []rune(s)
	t.cursor = len(t.input)
}

func (t *Task) submit(ctx *kernel.Context) {
	line := strings.TrimSpace(string(t.input))
	t.input = t.input[:0]
	t.cursor = 0

	if line == "" {
		t.histPos = len(t.history)
		return
	}

	if len(t.history) == 0 || t.history[len(t.history)-1] != line {
		t.history = append(t.history, line)
	}
	t.histPos = len(t.history)

	act, err := parseInput(line)
	if err != nil {
		t.appendLine(line)
		t.appendLine("error: " + err.Error())
		return
	}

	switch act.kind {
	case actionAssignVar:
		v, err := act.expr.Eval(t.e)
		if err != nil {
			t.appendLine(line)
			t.appendLine("error: " + err.Error())
			return
		}
		t.e.vars[act.varName] = v
		t.appendLine(fmt.Sprintf("%s = %g", act.varName, v))
		t.setGraphFromExpr(line, act.expr)
	case actionAssignFunc:
		t.e.funcs[act.funcName] = userFunc{param: act.funcParam, body: act.expr}
		t.appendLine(fmt.Sprintf("%s(%s) = ...", act.funcName, act.funcParam))
	case actionEval:
		v, err := act.expr.Eval(t.e)
		if err != nil {
			t.appendLine(line)
			t.appendLine("error: " + err.Error())
			return
		}
		t.appendLine(fmt.Sprintf("%s = %g", line, v))
		t.setGraphFromExpr(line, act.expr)
	default:
	}
}

func (t *Task) setGraphFromExpr(src string, ex node) {
	t.graphExpr = src
	t.graph = ex
	if strings.Contains(src, "x") {
		t.autoscaleY()
	}
}

func (t *Task) toggleGraph(ctx *kernel.Context) {
	_ = ctx
	if t.graph == nil {
		t.setMessage("graph: no expression")
		return
	}
	if t.mode == modeGraph {
		t.mode = modeCalc
		return
	}
	t.mode = modeGraph
	if t.xMin >= t.xMax {
		t.xMin, t.xMax = -10, 10
	}
	if t.yMin >= t.yMax {
		t.yMin, t.yMax = -10, 10
	}
	t.autoscaleY()
}

func (t *Task) handleGraphKey(ctx *kernel.Context, k key) {
	_ = ctx
	switch k.kind {
	case keyEsc:
		t.mode = modeCalc
	case keyRune:
		switch k.r {
		case 'q':
			t.mode = modeCalc
		case 'c':
			t.mode = modeCalc
		case 'a':
			t.autoscaleY()
		case '+', '=':
			t.zoom(0.8)
		case '-':
			t.zoom(1.25)
		}
	case keyLeft:
		t.pan(-0.1, 0)
	case keyRight:
		t.pan(0.1, 0)
	case keyUp:
		t.pan(0, 0.1)
	case keyDown:
		t.pan(0, -0.1)
	}
}

func (t *Task) pan(dxFrac, dyFrac float64) {
	dx := (t.xMax - t.xMin) * dxFrac
	dy := (t.yMax - t.yMin) * dyFrac
	t.xMin += dx
	t.xMax += dx
	t.yMin += dy
	t.yMax += dy
}

func (t *Task) zoom(factor float64) {
	cx := (t.xMin + t.xMax) / 2
	cy := (t.yMin + t.yMax) / 2
	hx := (t.xMax - t.xMin) / 2 * factor
	hy := (t.yMax - t.yMin) / 2 * factor
	if hx <= 0 || hy <= 0 {
		return
	}
	t.xMin = cx - hx
	t.xMax = cx + hx
	t.yMin = cy - hy
	t.yMax = cy + hy
}

func (t *Task) evalGraph(x float64) (float64, bool) {
	if t.graph == nil {
		return 0, false
	}
	prev, hadPrev := t.e.vars["x"]
	t.e.vars["x"] = x
	y, err := t.graph.Eval(t.e)
	if hadPrev {
		t.e.vars["x"] = prev
	} else {
		delete(t.e.vars, "x")
	}
	if err != nil {
		return 0, false
	}
	return y, true
}

func (t *Task) autoscaleY() {
	if t.graph == nil || t.xMin >= t.xMax {
		return
	}
	minY := math.Inf(1)
	maxY := math.Inf(-1)
	const samples = 240
	for i := 0; i < samples; i++ {
		x := t.xMin + (float64(i)/float64(samples-1))*(t.xMax-t.xMin)
		y, ok := t.evalGraph(x)
		if !ok || math.IsNaN(y) || math.IsInf(y, 0) {
			continue
		}
		if y < minY {
			minY = y
		}
		if y > maxY {
			maxY = y
		}
	}
	if !math.IsInf(minY, 0) && !math.IsInf(maxY, 0) && minY < maxY {
		pad := (maxY - minY) * 0.1
		if pad == 0 {
			pad = 1
		}
		t.yMin = minY - pad
		t.yMax = maxY + pad
	}
}

func (t *Task) appendLine(s string) {
	if s == "" {
		return
	}
	t.lines = append(t.lines, s)
	if len(t.lines) > 200 {
		t.lines = t.lines[len(t.lines)-200:]
	}
}

func (t *Task) histUp() {
	if len(t.history) == 0 {
		return
	}
	if t.histPos > 0 {
		t.histPos--
	}
	if t.histPos >= 0 && t.histPos < len(t.history) {
		t.setInput(t.history[t.histPos])
	}
}

func (t *Task) histDown() {
	if len(t.history) == 0 {
		return
	}
	if t.histPos < len(t.history) {
		t.histPos++
	}
	if t.histPos == len(t.history) {
		t.setInput("")
		return
	}
	if t.histPos >= 0 && t.histPos < len(t.history) {
		t.setInput(t.history[t.histPos])
	}
}

func (t *Task) headerText() string {
	if t.mode == modeGraph {
		if t.graphExpr == "" {
			return "VECTOR graph"
		}
		return "VECTOR graph: " + clipRunes(t.graphExpr, t.cols-13)
	}
	return "VECTOR"
}

func (t *Task) statusText() string {
	prefix := "> "
	line := string(t.input)
	msg := prefix + line
	if t.message != "" {
		msg = t.message + " | " + msg
	}
	return clipRunes(msg, t.cols)
}

func clipRunes(s string, max int) string {
	rs := []rune(s)
	if len(rs) > max {
		rs = rs[:max]
	}
	return string(rs)
}
