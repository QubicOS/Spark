package vector

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"spark/hal"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"

	"tinygo.org/x/tinyfont"
)

type tab uint8

const (
	tabTerminal tab = iota
	tabPlot
	tabStack
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

	tab tab

	lines []string

	input  []rune
	cursor int

	history []string
	histPos int

	inbuf []byte

	showHelp bool
	helpTop  int

	message string
	editVar string

	graphExpr string
	graph     node

	plots []plot

	xMin float64
	xMax float64
	yMin float64
	yMax float64

	stackSel int
	stackTop int
}

func New(disp hal.Display, ep kernel.Capability) *Task {
	return &Task{
		disp: disp,
		ep:   ep,
		e:    newEnv(),
		tab:  tabTerminal,
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
	t.appendLine("Commands: :exact :float :prec N")

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
	t.setMessage("F1 term | F2 plot | F3 stack | H help | q quit")
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
	switch k.kind {
	case keyF1:
		t.switchTab(tabTerminal)
		return
	case keyF2:
		t.switchTab(tabPlot)
		return
	case keyF3:
		t.switchTab(tabStack)
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

	if t.editVar != "" {
		t.handleEditKey(ctx, k)
		return
	}

	switch k.kind {
	case keyEsc:
		t.requestExit(ctx)
	case keyEnter:
		if t.tab == tabTerminal {
			t.submit(ctx)
		}
	case keyBackspace:
		if t.tab == tabTerminal {
			t.backspace()
		}
	case keyDelete:
		if t.tab == tabTerminal {
			t.deleteForward()
		}
	case keyLeft:
		switch t.tab {
		case tabTerminal:
			if t.cursor > 0 {
				t.cursor--
			}
		case tabPlot:
			t.handlePlotKey(ctx, k)
		case tabStack:
			t.handleStackKey(ctx, k)
		}
	case keyRight:
		switch t.tab {
		case tabTerminal:
			if t.cursor < len(t.input) {
				t.cursor++
			}
		case tabPlot:
			t.handlePlotKey(ctx, k)
		case tabStack:
			t.handleStackKey(ctx, k)
		}
	case keyHome:
		switch t.tab {
		case tabTerminal:
			t.cursor = 0
		case tabStack:
			t.handleStackKey(ctx, k)
		}
	case keyEnd:
		switch t.tab {
		case tabTerminal:
			t.cursor = len(t.input)
		case tabStack:
			t.handleStackKey(ctx, k)
		}
	case keyUp:
		switch t.tab {
		case tabTerminal:
			t.histUp()
		case tabPlot:
			t.handlePlotKey(ctx, k)
		case tabStack:
			t.handleStackKey(ctx, k)
		}
	case keyDown:
		switch t.tab {
		case tabTerminal:
			t.histDown()
		case tabPlot:
			t.handlePlotKey(ctx, k)
		case tabStack:
			t.handleStackKey(ctx, k)
		}
	case keyPageUp, keyPageDown:
		switch t.tab {
		case tabPlot:
			t.handlePlotKey(ctx, k)
		case tabStack:
			t.handleStackKey(ctx, k)
		}
	case keyRune:
		if k.r == 'q' {
			t.requestExit(ctx)
			return
		}
		switch t.tab {
		case tabTerminal:
			switch k.r {
			case 'g':
				t.switchTab(tabPlot)
			default:
				if k.r >= 0x20 && k.r != 0x7f {
					t.insertRune(k.r)
				}
			}
		case tabPlot:
			t.handlePlotKey(ctx, k)
		case tabStack:
			t.handleStackKey(ctx, k)
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
	t.evalLine(ctx, line, true)
}

func (t *Task) handleCommand(cmd string) {
	switch {
	case cmd == "term":
		t.switchTab(tabTerminal)
	case cmd == "plot":
		t.switchTab(tabPlot)
	case cmd == "stack":
		t.switchTab(tabStack)
	case cmd == "clear":
		t.lines = nil
		t.setMessage("cleared")
	case cmd == "exact":
		t.e.mode = modeExact
		t.setMessage("mode: exact")
	case cmd == "float":
		t.e.mode = modeFloat
		t.setMessage("mode: float")
	case strings.HasPrefix(cmd, "prec"):
		parts := strings.Fields(cmd)
		if len(parts) != 2 {
			t.setMessage("usage: :prec N")
			return
		}
		n, err := strconv.Atoi(parts[1])
		if err != nil || n < 1 || n > 32 {
			t.setMessage("prec: 1..32")
			return
		}
		t.e.prec = n
		t.setMessage(fmt.Sprintf("prec: %d", n))
	case cmd == "plotclear":
		t.plots = nil
		t.setMessage("plots cleared")
	case cmd == "plots":
		if len(t.plots) == 0 {
			t.appendLine("plots: (none)")
			return
		}
		for i, p := range t.plots {
			t.appendLine(fmt.Sprintf("plot[%d]: %s", i, p.src))
		}
	default:
		t.setMessage("unknown command: :" + cmd)
	}
}

func (t *Task) setGraphFromExpr(src string, ex node) {
	t.graphExpr = src
	t.graph = ex
	if nodeHasIdent(ex, "x") {
		t.autoscalePlots()
	}
}

type plot struct {
	src  string
	expr node
	xs   []float64
	ys   []float64
}

func (t *Task) addPlotFunc(src string, ex node) {
	if ex == nil {
		return
	}
	if len(t.plots) > 0 {
		last := t.plots[len(t.plots)-1]
		if last.src == src {
			return
		}
	}
	if len(t.plots) >= 8 {
		t.plots = t.plots[1:]
	}
	t.plots = append(t.plots, plot{src: src, expr: ex})
}

func (t *Task) addPlotSeries(src string, xs, ys []float64) {
	if len(xs) == 0 || len(xs) != len(ys) {
		return
	}
	if len(t.plots) >= 8 {
		t.plots = t.plots[1:]
	}
	t.plots = append(t.plots, plot{src: src, xs: xs, ys: ys})
}

func (t *Task) handlePlotKey(ctx *kernel.Context, k key) {
	_ = ctx
	switch k.kind {
	case keyRune:
		switch k.r {
		case 'c':
			t.switchTab(tabTerminal)
		case 'a':
			t.autoscalePlots()
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

func (t *Task) evalGraphFor(expr node, x float64) (float64, bool) {
	if expr == nil {
		return 0, false
	}
	prev, hadPrev := t.e.vars["x"]
	t.e.vars["x"] = NumberValue(Float(x))
	yv, err := expr.Eval(t.e)
	if hadPrev {
		t.e.vars["x"] = prev
	} else {
		delete(t.e.vars, "x")
	}
	if err != nil {
		return 0, false
	}
	if !yv.IsNumber() {
		return 0, false
	}
	return yv.num.Float64(), true
}

func (t *Task) autoscalePlots() {
	if len(t.plots) > 0 {
		t.autoscaleFromSeries()
		return
	}
	t.autoscaleFunc()
}

func (t *Task) autoscaleFromSeries() {
	minX := math.Inf(1)
	maxX := math.Inf(-1)
	minY := math.Inf(1)
	maxY := math.Inf(-1)
	for _, p := range t.plots {
		if len(p.xs) == 0 || len(p.ys) == 0 {
			continue
		}
		for i := range p.xs {
			x := p.xs[i]
			y := p.ys[i]
			if math.IsNaN(x) || math.IsInf(x, 0) || math.IsNaN(y) || math.IsInf(y, 0) {
				continue
			}
			if x < minX {
				minX = x
			}
			if x > maxX {
				maxX = x
			}
			if y < minY {
				minY = y
			}
			if y > maxY {
				maxY = y
			}
		}
	}
	if !math.IsInf(minX, 0) && !math.IsInf(maxX, 0) && minX < maxX {
		t.xMin, t.xMax = minX, maxX
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

func (t *Task) autoscaleFunc() {
	if t.graph == nil || t.xMin >= t.xMax {
		return
	}
	minY := math.Inf(1)
	maxY := math.Inf(-1)
	const samples = 240
	for i := 0; i < samples; i++ {
		x := t.xMin + (float64(i)/float64(samples-1))*(t.xMax-t.xMin)
		y, ok := t.evalGraphFor(t.graph, x)
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
	switch t.tab {
	case tabPlot:
		if t.graphExpr == "" {
			return "VECTOR plot | F1 term F2 plot* F3 stack"
		}
		head := "VECTOR plot | F1 term F2 plot* F3 stack | "
		return head + clipRunes(t.graphExpr, t.cols-len([]rune(head)))
	case tabStack:
		return "VECTOR stack | F1 term F2 plot F3 stack*"
	default:
		return "VECTOR | F1 term* F2 plot F3 stack"
	}
}

func (t *Task) statusText() string {
	var base string
	if t.editVar != "" {
		base = "edit " + t.editVar + " = " + string(t.input)
	} else {
		switch t.tab {
		case tabTerminal:
			base = "> " + string(t.input)
		case tabPlot:
			base = fmt.Sprintf("x:[%g..%g] y:[%g..%g]", t.xMin, t.xMax, t.yMin, t.yMax)
		case tabStack:
			base = "stack: Up/Down select | Enter edit | F1/F2 tabs"
		}
	}
	if t.message == "" {
		return clipRunes(base, t.cols)
	}
	return clipRunes(base+" | "+t.message, t.cols)
}

func (t *Task) formatValue(v Value) string {
	if v.kind == valueExpr {
		return NodeString(v.expr)
	}
	if v.kind == valueArray {
		if len(v.arr) == 0 {
			return "[]"
		}
		min := v.arr[0]
		max := v.arr[0]
		for _, x := range v.arr[1:] {
			if x < min {
				min = x
			}
			if x > max {
				max = x
			}
		}
		return fmt.Sprintf("[%d] %.*g..%.*g", len(v.arr), 6, min, 6, max)
	}
	return v.num.String(t.e.prec)
}

func (t *Task) setDomainFromArray(xs []float64) {
	if len(xs) < 2 {
		return
	}
	min := xs[0]
	max := xs[0]
	for _, x := range xs[1:] {
		if x < min {
			min = x
		}
		if x > max {
			max = x
		}
	}
	if min < max {
		t.xMin, t.xMax = min, max
	}
}

func (t *Task) tryPlotSeries(label string, v Value) {
	if !v.IsArray() {
		return
	}
	xv, ok := t.e.vars["x"]
	if !ok || !xv.IsArray() {
		return
	}
	if len(xv.arr) != len(v.arr) {
		return
	}
	t.addPlotSeries(label, append([]float64(nil), xv.arr...), append([]float64(nil), v.arr...))
	t.autoscalePlots()
}

func (t *Task) switchTab(newTab tab) {
	if newTab == t.tab && t.editVar == "" {
		return
	}
	if t.editVar != "" {
		t.editVar = ""
		t.input = t.input[:0]
		t.cursor = 0
	}

	t.tab = newTab
	switch t.tab {
	case tabTerminal:
		t.setMessage("Enter eval | g/F2 plot | F3 stack | :clear")
	case tabPlot:
		if t.graph == nil && len(t.plots) == 0 {
			t.setMessage("no plot yet (enter sin(x) then g/F2)")
			return
		}
		if t.xMin >= t.xMax {
			t.xMin, t.xMax = -10, 10
		}
		if t.yMin >= t.yMax {
			t.yMin, t.yMax = -10, 10
		}
		if len(t.plots) == 0 && t.graph != nil && nodeHasIdent(t.graph, "x") {
			t.addPlotFunc(t.graphExpr, t.graph)
		}
		t.autoscalePlots()
		t.setMessage("arrows pan | +/- zoom | a autoscale | F1 term | F3 stack")
	case tabStack:
		t.stackSel = 0
		t.stackTop = 0
		t.setMessage("Up/Down select | Enter edit | F1 term | F2 plot")
	}
}

func (t *Task) evalLine(ctx *kernel.Context, line string, recordHistory bool) {
	_ = ctx
	line = strings.TrimSpace(line)
	if line == "" {
		if recordHistory {
			t.histPos = len(t.history)
		}
		return
	}

	if strings.HasPrefix(line, ":") {
		t.handleCommand(strings.TrimSpace(line[1:]))
		return
	}

	if recordHistory {
		if len(t.history) == 0 || t.history[len(t.history)-1] != line {
			t.history = append(t.history, line)
		}
		t.histPos = len(t.history)
	}

	acts, err := parseInput(line)
	if err != nil {
		t.appendLine(line)
		t.appendLine("error: " + err.Error())
		return
	}

	t.appendLine(line)
	for _, act := range acts {
		switch act.kind {
		case actionAssignVar:
			v, err := act.expr.Eval(t.e)
			if err != nil {
				t.appendLine("error: " + err.Error())
				return
			}
			t.e.vars[act.varName] = v
			t.appendLine(fmt.Sprintf("%s = %s", act.varName, t.formatValue(v)))
			if v.IsArray() {
				if act.varName == "x" {
					t.setDomainFromArray(v.arr)
				}
				t.tryPlotSeries(act.varName, v)
			} else {
				t.setGraphFromExpr(act.varName, v.ToNode())
			}

		case actionAssignFunc:
			t.e.funcs[act.funcName] = userFunc{param: act.funcParam, body: act.expr}
			t.appendLine(fmt.Sprintf("%s(%s) = %s", act.funcName, act.funcParam, NodeString(act.expr)))

		case actionEval:
			v, err := act.expr.Eval(t.e)
			if err != nil {
				t.appendLine("error: " + err.Error())
				return
			}
			t.appendLine("= " + t.formatValue(v))
			if v.IsArray() {
				t.tryPlotSeries("result", v)
			} else {
				t.setGraphFromExpr(NodeString(act.expr), v.ToNode())
				if nodeHasIdent(act.expr, "x") {
					t.addPlotFunc(NodeString(act.expr), act.expr)
				}
			}
		}
	}
}

func (t *Task) handleStackKey(ctx *kernel.Context, k key) {
	_ = ctx
	vars := t.stackVars()
	if len(vars) == 0 {
		return
	}

	switch k.kind {
	case keyUp:
		if t.stackSel > 0 {
			t.stackSel--
		}
	case keyDown:
		if t.stackSel < len(vars)-1 {
			t.stackSel++
		}
	case keyHome:
		t.stackSel = 0
	case keyEnd:
		t.stackSel = len(vars) - 1
	case keyEnter:
		name := vars[t.stackSel]
		t.editVar = name
		t.setInput(t.valueEditString(t.e.vars[name]))
		t.setMessage("Enter apply | Esc cancel")
	case keyRune:
		if k.r == 'e' {
			name := vars[t.stackSel]
			t.editVar = name
			t.setInput(t.valueEditString(t.e.vars[name]))
			t.setMessage("Enter apply | Esc cancel")
		}
	}

	if t.stackSel < t.stackTop {
		t.stackTop = t.stackSel
	}
	if t.stackSel >= t.stackTop+t.viewRows {
		t.stackTop = t.stackSel - (t.viewRows - 1)
	}
	if t.stackTop < 0 {
		t.stackTop = 0
	}
	maxTop := len(vars) - t.viewRows
	if maxTop < 0 {
		maxTop = 0
	}
	if t.stackTop > maxTop {
		t.stackTop = maxTop
	}
}

func (t *Task) handleEditKey(ctx *kernel.Context, k key) {
	switch k.kind {
	case keyEsc:
		t.editVar = ""
		t.input = t.input[:0]
		t.cursor = 0
		t.setMessage("edit canceled")
	case keyEnter:
		exprText := strings.TrimSpace(string(t.input))
		name := t.editVar
		t.editVar = ""
		t.input = t.input[:0]
		t.cursor = 0
		t.evalLine(ctx, name+"="+exprText, true)
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
	case keyRune:
		if k.r >= 0x20 && k.r != 0x7f {
			t.insertRune(k.r)
		}
	}
}

func (t *Task) stackVars() []string {
	out := make([]string, 0, len(t.e.vars))
	for k := range t.e.vars {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (t *Task) valueEditString(v Value) string {
	switch v.kind {
	case valueExpr:
		return NodeString(v.expr)
	case valueArray:
		return ""
	default:
		return v.num.String(t.e.prec)
	}
}

func clipRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	rs := []rune(s)
	if len(rs) > max {
		rs = rs[:max]
	}
	return string(rs)
}
