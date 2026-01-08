package vector

// This file implements the Vector app task and UI logic.

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"spark/hal"
	vfsclient "spark/sparkos/client/vfs"
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

const (
	maxOutputLines    = 200
	maxHistoryEntries = 200
	maxPlots          = 8
	maxPlotPoints     = 1024
)

const (
	notebookDir  = "/vector"
	notebooksDir = "/vector/notebooks"
	notebookExt  = ".vnb"
	maxNotebook  = 64 * 1024
)

// Task implements a framebuffer-based math calculator with graphing.
type Task struct {
	disp hal.Display
	ep   kernel.Capability

	vfsCap kernel.Capability
	vfs    *vfsclient.Client

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
	hint    string
	ghost   string
	cands   []string
	best    string
	editVar string

	graphExpr string
	graph     node

	plots []plot

	xMin float64
	xMax float64
	yMin float64
	yMax float64

	plotDim       int
	plotYaw       float64
	plotPitch     float64
	plotZoom      float64
	plotColorMode uint8
	showAxes3D    bool

	stackSel int
	stackTop int

	zoomInFactor  float64
	zoomOutFactor float64
}

func New(disp hal.Display, ep kernel.Capability, vfsCap kernel.Capability) *Task {
	t := &Task{
		disp:   disp,
		ep:     ep,
		vfsCap: vfsCap,
		e:      newEnv(),
		tab:    tabTerminal,
		xMin:   -10,
		xMax:   10,
		yMin:   -10,
		yMax:   10,

		plotDim:   2,
		plotYaw:   default3DYaw,
		plotPitch: default3DPitch,
		plotZoom:  default3DZoom,

		plotColorMode: default3DPlotColorMode,

		zoomInFactor:  0.8,
		zoomOutFactor: 1.25,
	}
	t.seedPlotVars()
	return t
}

const (
	default3DYaw   = 0.8
	default3DPitch = 0.85
	default3DZoom  = 1.1
	// 0=mono, 1=height, 2=position.
	default3DPlotColorMode = 1
)

func (t *Task) reset3DView() {
	t.plotYaw = default3DYaw
	t.plotPitch = default3DPitch
	t.plotZoom = default3DZoom
	if t.plotColorMode > 2 {
		t.plotColorMode = default3DPlotColorMode
	}
}

func (t *Task) seedPlotVars() {
	if t.e == nil {
		return
	}
	if _, ok := t.e.vars["x"]; !ok {
		if v, err := builtinRange([]Value{
			NumberValue(Float(t.xMin)),
			NumberValue(Float(t.xMax)),
			NumberValue(Float(256)),
		}); err == nil && v.kind == valueArray {
			t.e.vars["x"] = v
		}
	}
	if _, ok := t.e.vars["y"]; !ok {
		if v, err := builtinRange([]Value{
			NumberValue(Float(t.yMin)),
			NumberValue(Float(t.yMax)),
			NumberValue(Float(256)),
		}); err == nil && v.kind == valueArray {
			t.e.vars["y"] = v
		}
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

	t.initSession()

	for msg := range ch {
		switch proto.Kind(msg.Kind) {
		case proto.MsgAppShutdown:
			t.unloadSession()
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
		if !active {
			t.unloadSession()
		}
		return
	}
	t.active = active
	if !t.active {
		t.unloadSession()
		return
	}
	if t.vfs == nil && t.vfsCap.Valid() {
		t.vfs = vfsclient.New(t.vfsCap)
	}
	t.initSession()
	t.setMessage("F1 term | F2 plot | F3 stack | H help | q quit")
	t.updateHint()
	t.render()
}

func (t *Task) setMessage(msg string) {
	t.message = msg
}

func (t *Task) setHint(hint string) {
	t.hint = hint
}

func (t *Task) requestExit(ctx *kernel.Context) {
	if t.fb != nil {
		t.fb.ClearRGB(0, 0, 0)
		_ = t.fb.Present()
	}
	t.active = false
	t.showHelp = false
	t.unloadSession()

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

func (t *Task) initSession() {
	if t.e == nil {
		t.e = newEnv()
	}
	t.seedPlotVars()
	if len(t.lines) != 0 {
		return
	}

	t.appendLine("V   V Vector: calculator + graph + 2D/3D plotter")
	t.appendLine("V   V Enter `sin(x)` (2D) or `sin(x)*cos(y)` (3D), then press Enter")
	t.appendLine(" V V  Plot: press `g` or go to F2 | 3D: `$plotdim 3`, arrows rotate, +/- zoom")
	t.appendLine("  V   Commands: :help :exact :float :prec N :autoscale :resetview :save :load :notebooks")
}

func (t *Task) unloadSession() {
	t.e = newEnv()

	t.tab = tabTerminal

	t.lines = nil

	t.input = nil
	t.cursor = 0

	t.history = nil
	t.histPos = 0

	t.inbuf = nil

	t.showHelp = false
	t.helpTop = 0

	t.message = ""
	t.hint = ""
	t.ghost = ""

	t.cands = nil
	t.best = ""
	t.editVar = ""

	t.graphExpr = ""
	t.graph = nil
	t.plots = nil

	t.xMin = -10
	t.xMax = 10
	t.yMin = -10
	t.yMax = 10

	t.plotDim = 2
	t.reset3DView()

	t.stackSel = 0
	t.stackTop = 0

	t.vfs = nil

	t.seedPlotVars()
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

	// Avoid letter hotkeys while typing in the REPL.
	if t.tab != tabTerminal && k.kind == keyRune && (k.r == 'H' || k.r == 'h') {
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
	case keyTab:
		switch t.tab {
		case tabTerminal:
			t.autocomplete()
		case tabPlot:
			t.handlePlotKey(ctx, k)
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
				t.updateHint()
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
				t.updateHint()
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
			t.updateHint()
		case tabStack:
			t.handleStackKey(ctx, k)
		}
	case keyEnd:
		switch t.tab {
		case tabTerminal:
			t.cursor = len(t.input)
			t.updateHint()
		case tabStack:
			t.handleStackKey(ctx, k)
		}
	case keyUp:
		switch t.tab {
		case tabTerminal:
			t.histUp()
			t.updateHint()
		case tabPlot:
			t.handlePlotKey(ctx, k)
		case tabStack:
			t.handleStackKey(ctx, k)
		}
	case keyDown:
		switch t.tab {
		case tabTerminal:
			t.histDown()
			t.updateHint()
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
	case keyCtrl:
		switch t.tab {
		case tabTerminal:
			if k.ctrl == 0x07 {
				t.switchTab(tabPlot)
			}
		}
	case keyRune:
		if t.tab != tabTerminal && k.r == 'q' {
			t.requestExit(ctx)
			return
		}
		switch t.tab {
		case tabTerminal:
			if k.r >= 0x20 && k.r != 0x7f {
				t.insertRune(k.r)
				t.updateHint()
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
	t.updateHint()
}

func (t *Task) deleteForward() {
	if t.cursor < 0 || t.cursor >= len(t.input) {
		return
	}
	copy(t.input[t.cursor:], t.input[t.cursor+1:])
	t.input = t.input[:len(t.input)-1]
	t.updateHint()
}

func (t *Task) setInput(s string) {
	t.input = []rune(s)
	t.cursor = len(t.input)
	t.updateHint()
}

func (t *Task) submit(ctx *kernel.Context) {
	line := strings.TrimSpace(string(t.input))
	t.input = t.input[:0]
	t.cursor = 0
	t.ghost = ""
	t.hint = ""
	t.cands = nil
	t.best = ""
	t.evalLine(ctx, line, true)
}

func (t *Task) handleCommand(ctx *kernel.Context, cmdline string) {
	fields := strings.Fields(cmdline)
	if len(fields) == 0 {
		return
	}
	cmd := fields[0]

	switch cmd {
	case "save":
		if len(fields) != 2 {
			t.setMessage("usage: :save NAME|/path/file.vnb")
			return
		}
		if err := t.saveNotebook(ctx, fields[1]); err != nil {
			t.setMessage("save: " + err.Error())
			return
		}
		t.setMessage("saved")
	case "load":
		if len(fields) != 2 {
			t.setMessage("usage: :load NAME|/path/file.vnb")
			return
		}
		if err := t.loadNotebook(ctx, fields[1]); err != nil {
			t.setMessage("load: " + err.Error())
			return
		}
		t.setMessage("loaded")
	case "notebooks":
		if err := t.listNotebooks(ctx); err != nil {
			t.setMessage("notebooks: " + err.Error())
			return
		}
	case "new":
		t.unloadSession()
		if t.vfs == nil && t.vfsCap.Valid() {
			t.vfs = vfsclient.New(t.vfsCap)
		}
		t.initSession()
		t.setMessage("new notebook")

	case "term":
		t.switchTab(tabTerminal)
	case "plot":
		t.forcePlot()
		t.switchTab(tabPlot)
	case "stack":
		t.switchTab(tabStack)
	case "help":
		t.showHelp = !t.showHelp
		if t.showHelp {
			t.setMessage("help: on")
		} else {
			t.setMessage("help: off")
		}
	case "clear":
		t.lines = nil
		t.setMessage("cleared")
	case "exact":
		t.e.mode = modeExact
		t.setMessage("mode: exact")
	case "float":
		t.e.mode = modeFloat
		t.setMessage("mode: float")
	case "prec":
		if len(fields) != 2 {
			t.setMessage("usage: :prec N")
			return
		}
		n, err := strconv.Atoi(fields[1])
		if err != nil || n < 1 || n > 32 {
			t.setMessage("prec: 1..32")
			return
		}
		t.e.prec = n
		t.setMessage(fmt.Sprintf("prec: %d", n))
	case "plotclear":
		t.plots = nil
		t.setMessage("plots cleared")
	case "plots":
		if len(t.plots) == 0 {
			t.appendLine("plots: (none)")
			return
		}
		for i, p := range t.plots {
			t.appendLine(fmt.Sprintf("plot[%d]: %s", i, p.src))
		}
	case "plotdel":
		if len(fields) != 2 {
			t.setMessage("usage: :plotdel N")
			return
		}
		n, err := strconv.Atoi(fields[1])
		if err != nil {
			t.setMessage("usage: :plotdel N")
			return
		}
		i := n
		if i < 0 || i >= len(t.plots) {
			alt := n - 1
			if alt >= 0 && alt < len(t.plots) {
				i = alt
			}
		}
		if i < 0 || i >= len(t.plots) {
			t.setMessage("plot index out of range")
			return
		}
		t.plots = append(t.plots[:i], t.plots[i+1:]...)
		t.setMessage(fmt.Sprintf("plot deleted: %d", i))
	case "x", "xrange", "domain":
		if len(fields) != 3 {
			t.setMessage("usage: :x A B")
			return
		}
		a, err := strconv.ParseFloat(fields[1], 64)
		if err != nil {
			t.setMessage("usage: :x A B")
			return
		}
		b, err := strconv.ParseFloat(fields[2], 64)
		if err != nil {
			t.setMessage("usage: :x A B")
			return
		}
		if a >= b {
			t.setMessage("x: A must be < B")
			return
		}
		t.xMin, t.xMax = a, b
		t.normalizeView()
		t.setMessage(fmt.Sprintf("x: [%s, %s]", formatFloat(t.xMin, t.e.prec), formatFloat(t.xMax, t.e.prec)))
	case "y", "yrange":
		if len(fields) != 3 {
			t.setMessage("usage: :y A B")
			return
		}
		a, err := strconv.ParseFloat(fields[1], 64)
		if err != nil {
			t.setMessage("usage: :y A B")
			return
		}
		b, err := strconv.ParseFloat(fields[2], 64)
		if err != nil {
			t.setMessage("usage: :y A B")
			return
		}
		if a >= b {
			t.setMessage("y: A must be < B")
			return
		}
		t.yMin, t.yMax = a, b
		t.normalizeView()
		t.setMessage(fmt.Sprintf("y: [%s, %s]", formatFloat(t.yMin, t.e.prec), formatFloat(t.yMax, t.e.prec)))
	case "view":
		if len(fields) != 5 {
			t.setMessage("usage: :view xmin xmax ymin ymax")
			return
		}
		xMin, err := strconv.ParseFloat(fields[1], 64)
		if err != nil {
			t.setMessage("usage: :view xmin xmax ymin ymax")
			return
		}
		xMax, err := strconv.ParseFloat(fields[2], 64)
		if err != nil {
			t.setMessage("usage: :view xmin xmax ymin ymax")
			return
		}
		yMin, err := strconv.ParseFloat(fields[3], 64)
		if err != nil {
			t.setMessage("usage: :view xmin xmax ymin ymax")
			return
		}
		yMax, err := strconv.ParseFloat(fields[4], 64)
		if err != nil {
			t.setMessage("usage: :view xmin xmax ymin ymax")
			return
		}
		if xMin >= xMax || yMin >= yMax {
			t.setMessage("view: min must be < max")
			return
		}
		t.xMin, t.xMax = xMin, xMax
		t.yMin, t.yMax = yMin, yMax
		t.normalizeView()
		t.setMessage("view updated")
	case "about", "autoscale", "resetview", "vars", "funcs", "del":
		t.handleServiceCommand(ctx, cmdline)
	default:
		t.setMessage("unknown command: :" + cmdline)
	}
}

func (t *Task) notebookPath(arg string) string {
	if strings.Contains(arg, "/") {
		if strings.HasSuffix(strings.ToLower(arg), notebookExt) {
			return arg
		}
		return arg + notebookExt
	}
	name := arg
	if strings.HasSuffix(strings.ToLower(name), notebookExt) {
		name = name[:len(name)-len(notebookExt)]
	}
	return notebooksDir + "/" + name + notebookExt
}

func (t *Task) ensureNotebooksDir(ctx *kernel.Context) error {
	if t.vfs == nil {
		return errors.New("vfs unavailable")
	}
	if err := t.vfs.Mkdir(ctx, notebookDir); err != nil {
		if typ, _, statErr := t.vfs.Stat(ctx, notebookDir); statErr != nil || typ != proto.VFSEntryDir {
			return err
		}
	}
	if err := t.vfs.Mkdir(ctx, notebooksDir); err != nil {
		if typ, _, statErr := t.vfs.Stat(ctx, notebooksDir); statErr != nil || typ != proto.VFSEntryDir {
			return err
		}
	}
	return nil
}

func (t *Task) saveNotebook(ctx *kernel.Context, arg string) error {
	if t.vfs == nil {
		return errors.New("vfs unavailable")
	}
	if err := t.ensureNotebooksDir(ctx); err != nil {
		return fmt.Errorf("ensure notebooks dir: %w", err)
	}
	path := t.notebookPath(arg)

	var b strings.Builder
	b.WriteString("# vector notebook v1\n")
	b.WriteString("# saved: ")
	b.WriteString(fmt.Sprintf("mode=%v prec=%d\n", t.e.mode, t.e.prec))
	for _, line := range t.history {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	data := []byte(b.String())
	if len(data) > maxNotebook {
		return fmt.Errorf("too large (%d bytes)", len(data))
	}
	_, err := t.vfs.Write(ctx, path, proto.VFSWriteTruncate, data)
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	t.appendLine(":save " + arg)
	t.appendLine("saved: " + path)
	return nil
}

func (t *Task) loadNotebook(ctx *kernel.Context, arg string) error {
	if t.vfs == nil {
		return errors.New("vfs unavailable")
	}
	path := t.notebookPath(arg)
	data, err := t.readAll(ctx, path, maxNotebook)
	if err != nil {
		return err
	}

	t.appendLine(":load " + arg)
	t.unloadSession()
	if t.vfs == nil && t.vfsCap.Valid() {
		t.vfs = vfsclient.New(t.vfsCap)
	}
	t.initSession()
	t.setMessage("loaded: " + path)

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		t.evalLine(ctx, line, true)
	}
	return nil
}

func (t *Task) listNotebooks(ctx *kernel.Context) error {
	if t.vfs == nil {
		return errors.New("vfs unavailable")
	}
	if err := t.ensureNotebooksDir(ctx); err != nil {
		return fmt.Errorf("ensure notebooks dir: %w", err)
	}
	ents, err := t.vfs.List(ctx, notebooksDir)
	if err != nil {
		return fmt.Errorf("list %s: %w", notebooksDir, err)
	}
	t.appendLine(":notebooks")
	found := false
	for _, e := range ents {
		if e.Type != proto.VFSEntryFile {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(e.Name), notebookExt) {
			continue
		}
		found = true
		t.appendLine(e.Name)
	}
	if !found {
		t.appendLine("(none)")
	}
	return nil
}

func (t *Task) readAll(ctx *kernel.Context, path string, maxBytes int) ([]byte, error) {
	if t.vfs == nil {
		return nil, errors.New("vfs unavailable")
	}
	typ, size, err := t.vfs.Stat(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	if typ != proto.VFSEntryFile {
		return nil, fmt.Errorf("not a file: %s", path)
	}
	if maxBytes > 0 && int(size) > maxBytes {
		return nil, fmt.Errorf("too large (%d bytes)", size)
	}

	out := make([]byte, 0, size)
	var off uint32
	for {
		chunk, eof, err := t.vfs.ReadAt(ctx, path, off, 1024)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		out = append(out, chunk...)
		off += uint32(len(chunk))
		if eof {
			return out, nil
		}
		if maxBytes > 0 && len(out) > maxBytes {
			return nil, fmt.Errorf("too large (> %d bytes)", maxBytes)
		}
	}
}

func (t *Task) handleServiceCommand(ctx *kernel.Context, cmdline string) {
	_ = ctx
	fields := strings.Fields(cmdline)
	if len(fields) == 0 {
		return
	}
	cmd := fields[0]

	t.appendLine("$" + cmdline)
	switch cmd {
	case "help":
		t.appendLine("service commands:")
		for _, s := range serviceCommands() {
			line := "$" + s
			if s == "plotdim" {
				line += " 2|3"
			}
			if s == "plotcolor" {
				line += " [0|1|2]"
			}
			t.appendLine("  " + line)
		}
	case "about":
		t.appendLine("Vector: CAS + plotter + REPL.")
	case "clear":
		t.lines = nil
		t.setMessage("cleared")
	case "plotdim":
		if len(fields) != 2 {
			t.appendLine("usage: $plotdim 2|3")
			return
		}
		n, err := strconv.Atoi(fields[1])
		if err != nil || (n != 2 && n != 3) {
			t.appendLine("plotdim: expected 2 or 3")
			return
		}
		t.plotDim = n
		if n == 3 {
			t.reset3DView()
		}
		if t.tab == tabPlot {
			t.setMessage(t.plotMessage())
		} else {
			t.setMessage(fmt.Sprintf("plotdim: %d", n))
		}
	case "plotcolor":
		// plotcolor [0|1|2] or plotcolor [mono|height|pos].
		if len(fields) == 1 {
			t.plotColorMode = (t.plotColorMode + 1) % 3
		} else if len(fields) == 2 {
			switch fields[1] {
			case "0", "mono":
				t.plotColorMode = 0
			case "1", "height":
				t.plotColorMode = 1
			case "2", "pos", "position":
				t.plotColorMode = 2
			default:
				t.appendLine("plotcolor: expected 0|1|2 or mono|height|pos")
				return
			}
		} else {
			t.appendLine("usage: $plotcolor [0|1|2]")
			return
		}
		name := "mono"
		switch t.plotColorMode {
		case 1:
			name = "height"
		case 2:
			name = "pos"
		}
		t.setMessage("plotcolor: " + name)
	case "resetview":
		t.xMin, t.xMax = -10, 10
		t.yMin, t.yMax = -10, 10
		t.normalizeView()
		t.reset3DView()
		t.setMessage("view reset")
	case "autoscale":
		t.autoscalePlots()
		t.setMessage("autoscale")
	case "vars":
		vars := t.stackVars()
		if len(vars) == 0 {
			t.appendLine("vars: (none)")
			return
		}
		for _, name := range vars {
			t.appendLine(name + " = " + t.formatValue(t.e.vars[name]))
		}
	case "funcs":
		if len(t.e.funcs) == 0 {
			t.appendLine("funcs: (none)")
			return
		}
		names := make([]string, 0, len(t.e.funcs))
		for name := range t.e.funcs {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			f := t.e.funcs[name]
			t.appendLine(fmt.Sprintf("%s(%s) = %s", name, f.param, NodeString(f.body)))
		}
	case "del":
		if len(fields) != 2 {
			t.appendLine("usage: $del name")
			return
		}
		name := fields[1]
		if _, ok := t.e.vars[name]; ok {
			delete(t.e.vars, name)
			t.appendLine("deleted var: " + name)
			return
		}
		if _, ok := t.e.funcs[name]; ok {
			delete(t.e.funcs, name)
			t.appendLine("deleted func: " + name)
			return
		}
		t.appendLine("not found: " + name)
	default:
		t.appendLine("unknown service command: $" + cmd)
	}
}

func (t *Task) forcePlot() {
	// 1) Prefer existing graph expression.
	if t.graph != nil && nodeHasIdent(t.graph, "x") {
		t.addPlotFunc(t.graphExpr, t.graph)
		return
	}

	// 2) If y is defined as an expression, plot it as y(x).
	if v, ok := t.e.vars["y"]; ok && v.kind == valueExpr && v.expr != nil && nodeHasIdent(v.expr, "x") {
		src := "y = " + NodeString(v.expr)
		t.setGraphFromExpr(src, v.expr)
		t.addPlotFunc(src, v.expr)
		return
	}

	// 3) If y is an array and x is an array, plot a series.
	if v, ok := t.e.vars["y"]; ok && v.kind == valueArray {
		t.tryPlotSeries("y", v)
	}
}

func serviceCommands() []string {
	return []string{
		"help",
		"about",
		"clear",
		"plotdim",
		"plotcolor",
		"resetview",
		"autoscale",
		"vars",
		"funcs",
		"del",
	}
}

func completeServiceFromPrefix(prefix string) []string {
	var out []string
	for _, s := range serviceCommands() {
		if strings.HasPrefix(s, prefix) {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func pickBestCompletion(prefix string, cands []string) string {
	for _, s := range cands {
		if s == prefix {
			return s
		}
	}
	best := cands[0]
	for _, s := range cands[1:] {
		if len(s) < len(best) || (len(s) == len(best) && s < best) {
			best = s
		}
	}
	return best
}

func (t *Task) setGraphFromExpr(src string, ex node) {
	t.graphExpr = src
	t.graph = ex
	if t.plotDim != 3 && nodeHasIdent(ex, "x") {
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
	if len(t.plots) >= maxPlots {
		t.plots = t.plots[1:]
	}
	t.plots = append(t.plots, plot{src: src, expr: ex})
}

func (t *Task) addPlotSeries(src string, xs, ys []float64) {
	if len(xs) == 0 || len(xs) != len(ys) {
		return
	}
	if len(xs) > maxPlotPoints {
		xs, ys = downsampleSeries(xs, ys, maxPlotPoints)
	}
	if len(t.plots) >= maxPlots {
		t.plots = t.plots[1:]
	}
	t.plots = append(t.plots, plot{src: src, xs: xs, ys: ys})
}

func downsampleSeries(xs, ys []float64, maxPoints int) ([]float64, []float64) {
	if maxPoints <= 1 || len(xs) <= maxPoints || len(xs) != len(ys) {
		return xs, ys
	}

	step := len(xs) / maxPoints
	if step < 1 {
		step = 1
	}

	outX := make([]float64, 0, maxPoints)
	outY := make([]float64, 0, maxPoints)
	for i := 0; i < len(xs) && len(outX) < maxPoints; i += step {
		outX = append(outX, xs[i])
		outY = append(outY, ys[i])
	}
	if len(outX) == 0 {
		return xs, ys
	}

	last := len(xs) - 1
	outX[len(outX)-1] = xs[last]
	outY[len(outY)-1] = ys[last]
	return outX, outY
}

func (t *Task) handlePlotKey(ctx *kernel.Context, k key) {
	_ = ctx
	switch k.kind {
	case keyTab:
		if t.plotDim == 3 {
			t.showAxes3D = !t.showAxes3D
			if t.showAxes3D {
				t.setMessage("3D axes: on")
			} else {
				t.setMessage("3D axes: off")
			}
		}
	case keyRune:
		switch k.r {
		case 'c':
			t.switchTab(tabTerminal)
		case 'a':
			t.autoscalePlots()
		case '+', '=':
			if t.plotDim == 3 {
				t.zoom3D(t.zoomInFactor)
			} else {
				t.zoom(t.zoomInFactor)
			}
		case '-':
			if t.plotDim == 3 {
				t.zoom3D(t.zoomOutFactor)
			} else {
				t.zoom(t.zoomOutFactor)
			}
		case 'z':
			t.cyclePlotZoomMode()
			t.setMessage(fmt.Sprintf("zoom: in x%0.2f out x%0.2f", t.zoomInFactor, t.zoomOutFactor))
		}
	case keyLeft:
		if t.plotDim == 3 {
			t.plotYaw -= 0.1
		} else {
			t.pan(-0.1, 0)
		}
	case keyRight:
		if t.plotDim == 3 {
			t.plotYaw += 0.1
		} else {
			t.pan(0.1, 0)
		}
	case keyUp:
		if t.plotDim == 3 {
			t.plotPitch = clampFloat(t.plotPitch-0.08, -1.2, 1.2)
		} else {
			t.pan(0, 0.1)
		}
	case keyDown:
		if t.plotDim == 3 {
			t.plotPitch = clampFloat(t.plotPitch+0.08, -1.2, 1.2)
		} else {
			t.pan(0, -0.1)
		}
	case keyPageUp:
		if t.plotDim == 3 {
			t.zoom3D(t.zoomInFactor)
		} else {
			t.zoom(t.zoomInFactor)
		}
	case keyPageDown:
		if t.plotDim == 3 {
			t.zoom3D(t.zoomOutFactor)
		} else {
			t.zoom(t.zoomOutFactor)
		}
	}
}

func (t *Task) pan(dxFrac, dyFrac float64) {
	dx := (t.xMax - t.xMin) * dxFrac
	dy := (t.yMax - t.yMin) * dyFrac
	t.xMin += dx
	t.xMax += dx
	t.yMin += dy
	t.yMax += dy
	t.normalizeView()
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
	t.normalizeView()
}

func (t *Task) zoom3D(factor float64) {
	if factor <= 0 || math.IsNaN(factor) || math.IsInf(factor, 0) {
		return
	}
	zoom := t.plotZoom / factor
	if zoom < 0.2 {
		zoom = 0.2
	}
	if zoom > 20 {
		zoom = 20
	}
	t.plotZoom = zoom
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

func (t *Task) evalSurfaceFor(expr node, x, y float64) (float64, bool) {
	if expr == nil {
		return 0, false
	}

	prevX, hadPrevX := t.e.vars["x"]
	prevY, hadPrevY := t.e.vars["y"]

	t.e.vars["x"] = NumberValue(Float(x))
	t.e.vars["y"] = NumberValue(Float(y))

	zv, err := expr.Eval(t.e)

	if hadPrevX {
		t.e.vars["x"] = prevX
	} else {
		delete(t.e.vars, "x")
	}
	if hadPrevY {
		t.e.vars["y"] = prevY
	} else {
		delete(t.e.vars, "y")
	}

	if err != nil {
		return 0, false
	}
	if !zv.IsNumber() {
		return 0, false
	}
	return zv.num.Float64(), true
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
	if len(t.lines) >= maxOutputLines {
		copy(t.lines, t.lines[1:])
		t.lines[len(t.lines)-1] = s
		return
	}
	t.lines = append(t.lines, s)
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
	if t.tab == tabTerminal && t.editVar == "" {
		var parts []string
		if t.hint != "" {
			parts = append(parts, t.hint)
		}
		if t.message != "" {
			parts = append(parts, t.message)
		}
		return clipRunes(strings.Join(parts, " | "), t.cols)
	}

	var base string
	if t.editVar != "" {
		base = "edit " + t.editVar + " = " + string(t.input)
	} else {
		switch t.tab {
		case tabTerminal:
			base = "> " + string(t.input)
		case tabPlot:
			if t.plotDim == 3 {
				base = fmt.Sprintf(
					"3D x:[%s..%s] y:[%s..%s] zoom:%0.2f",
					fmtAxis(t.xMin),
					fmtAxis(t.xMax),
					fmtAxis(t.yMin),
					fmtAxis(t.yMax),
					t.plotZoom,
				)
			} else {
				base = fmt.Sprintf(
					"x:[%s..%s] y:[%s..%s]",
					fmtAxis(t.xMin),
					fmtAxis(t.xMax),
					fmtAxis(t.yMin),
					fmtAxis(t.yMax),
				)
			}
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
	if v.kind == valueComplex {
		return formatComplex(v.c, t.e.prec)
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
	if v.kind == valueMatrix {
		if len(v.mat) == 0 || v.rows <= 0 || v.cols <= 0 {
			return "[?]"
		}
		min := v.mat[0]
		max := v.mat[0]
		for _, x := range v.mat[1:] {
			if x < min {
				min = x
			}
			if x > max {
				max = x
			}
		}
		return fmt.Sprintf("[%dx%d] %.*g..%.*g", v.rows, v.cols, 6, min, 6, max)
	}
	return v.num.String(t.e.prec)
}

func formatComplex(z complex128, prec int) string {
	re := real(z)
	im := imag(z)
	if im == 0 {
		return formatFloat(re, prec)
	}
	if re == 0 {
		return formatFloat(im, prec) + "i"
	}
	ims := formatFloat(im, prec)
	if im > 0 {
		ims = "+" + ims
	}
	return formatFloat(re, prec) + ims + "i"
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

func (t *Task) setRangeFromArray(ys []float64) {
	if len(ys) < 2 {
		return
	}
	min := ys[0]
	max := ys[0]
	for _, y := range ys[1:] {
		if y < min {
			min = y
		}
		if y > max {
			max = y
		}
	}
	if min < max {
		t.yMin, t.yMax = min, max
	}
}

func (t *Task) tryPlotSeries(label string, v Value) {
	if label == "x" {
		return
	}
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
		t.setMessage("Enter eval | Ctrl+G plot | F2 plot | F3 stack | :clear")
		t.updateHint()
	case tabPlot:
		t.hint = ""
		t.ghost = ""
		t.cands = nil
		t.best = ""
		if t.graph == nil && len(t.plots) == 0 {
			t.setMessage("no plot yet (enter sin(x) then Ctrl+G/F2)")
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
		if t.plotDim != 3 {
			t.autoscalePlots()
		}
		t.setMessage(t.plotMessage())
	case tabStack:
		t.hint = ""
		t.ghost = ""
		t.cands = nil
		t.best = ""
		t.stackSel = 0
		t.stackTop = 0
		t.setMessage("Up/Down select | Enter edit | F1 term | F2 plot")
	}
}

func (t *Task) plotMessage() string {
	if t.plotDim == 3 {
		return "3D: arrows rotate | +/- zoom | PgUp/PgDn zoom | Tab axes | z zoom step | a autoscale | c term | $plotdim 2"
	}
	return "arrows pan | +/- zoom | PgUp/PgDn zoom | z zoom step | a autoscale | F1 term | F3 stack"
}

func (t *Task) cyclePlotZoomMode() {
	switch {
	case t.zoomInFactor >= 0.89:
		t.zoomInFactor = 0.8
		t.zoomOutFactor = 1.25
	case t.zoomInFactor >= 0.79:
		t.zoomInFactor = 0.5
		t.zoomOutFactor = 2.0
	default:
		t.zoomInFactor = 0.9
		t.zoomOutFactor = 1.0 / 0.9
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
		t.handleCommand(ctx, strings.TrimSpace(line[1:]))
		return
	}
	if strings.HasPrefix(line, "$") {
		t.handleServiceCommand(ctx, strings.TrimSpace(line[1:]))
		return
	}

	if recordHistory {
		t.pushHistory(line)
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
				// Allow defining plot expressions like `y = x` in a fresh notebook.
				if errors.Is(err, ErrUnknownVar) && (act.varName == "y" || act.varName == "z") && nodeHasIdent(act.expr, "x") {
					ex := act.expr.Simplify()
					t.e.vars[act.varName] = ExprValue(ex)
					src := fmt.Sprintf("%s = %s", act.varName, NodeString(ex))
					t.appendLine(src)
					t.setGraphFromExpr(src, ex)
					if nodeHasIdent(ex, "x") {
						t.addPlotFunc(src, ex)
					}
					break
				}
				t.appendLine("error: " + err.Error())
				return
			}
			t.e.vars[act.varName] = v
			t.appendLine(fmt.Sprintf("%s = %s", act.varName, t.formatValue(v)))
			if v.IsArray() {
				if act.varName == "x" {
					t.setDomainFromArray(v.arr)
					break
				}
				if act.varName == "y" {
					t.setRangeFromArray(v.arr)
					t.tryPlotSeries("y", v)
					break
				}
				t.tryPlotSeries(act.varName, v)
			} else if v.IsMatrix() && v.cols == 2 {
				t.tryPlotMatrixXY(act.varName, v)
			} else {
				t.setGraphFromExpr(act.varName, v.ToNode())
			}

		case actionAssignFunc:
			t.e.funcs[act.funcName] = userFunc{param: act.funcParam, body: act.expr}
			t.appendLine(fmt.Sprintf("%s(%s) = %s", act.funcName, act.funcParam, NodeString(act.expr)))

		case actionEval:
			v, err := act.expr.Eval(t.e)
			if err != nil {
				// Allow plotting expressions with free variables (e.g. x, y) without requiring them to be defined.
				if errors.Is(err, ErrUnknownVar) && (nodeHasIdent(act.expr, "x") || nodeHasIdent(act.expr, "y")) {
					t.setGraphFromExpr(NodeString(act.expr), act.expr)
					t.appendLine("= " + NodeString(act.expr))
					break
				}
				t.appendLine("error: " + err.Error())
				return
			}
			t.appendLine("= " + t.formatValue(v))
			if v.IsArray() {
				// If the expression depends on y, treat it as a 3D surface even if x/y are arrays.
				if nodeHasIdent(act.expr, "y") {
					t.setGraphFromExpr(NodeString(act.expr), act.expr)
				}
				t.tryPlotSeries("result", v)
			} else if v.IsMatrix() && v.cols == 2 {
				t.tryPlotMatrixXY("result", v)
			} else {
				t.setGraphFromExpr(NodeString(act.expr), v.ToNode())
				if nodeHasIdent(act.expr, "x") {
					t.addPlotFunc(NodeString(act.expr), act.expr)
				}
			}
		}
	}
}

func (t *Task) tryPlotMatrixXY(label string, v Value) {
	if label == "" {
		return
	}
	if v.kind != valueMatrix || v.cols != 2 || v.rows <= 0 {
		return
	}
	xs := make([]float64, v.rows)
	ys := make([]float64, v.rows)
	for i := 0; i < v.rows; i++ {
		xs[i] = v.mat[i*2+0]
		ys[i] = v.mat[i*2+1]
	}
	t.addPlotSeries(label, xs, ys)
	t.autoscalePlots()
}

func (t *Task) pushHistory(line string) {
	if line == "" {
		return
	}
	if len(t.history) > 0 && t.history[len(t.history)-1] == line {
		return
	}
	if len(t.history) >= maxHistoryEntries {
		copy(t.history, t.history[1:])
		t.history[len(t.history)-1] = line
		return
	}
	t.history = append(t.history, line)
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

func (t *Task) updateHint() {
	t.hint = ""
	t.ghost = ""
	t.cands = nil
	t.best = ""

	if t.tab != tabTerminal {
		return
	}
	if t.editVar != "" {
		return
	}

	line := string(t.input)
	if strings.HasPrefix(line, ":") {
		t.updateCommandHint(line)
		return
	}
	if strings.HasPrefix(line, "$") {
		t.updateServiceHint(line)
		return
	}

	start, end := identBoundsAt(t.input, t.cursor)
	if start == end || t.cursor < start {
		return
	}

	prefix := string(t.input[start:t.cursor])
	if prefix == "" {
		return
	}
	cands := t.completeFromPrefix(prefix, false)
	if len(cands) == 0 {
		return
	}

	best := pickBestCompletion(prefix, cands)
	t.cands = cands
	t.best = best
	if best != prefix && end == len(t.input) && t.cursor == end && strings.HasPrefix(best, prefix) {
		t.ghost = best[len(prefix):]
	}

	switch prefix {
	case "range":
		t.hint = "range(a, b[, n])"
	case "diff":
		t.hint = "diff(expr, x)"
	case "simp":
		t.hint = "simp(expr)"
	case "expand":
		t.hint = "expand(expr)"
	case "series":
		t.hint = "series(expr, x, a, n)"
	case "horner":
		t.hint = "horner(expr, x)"
	case "degree":
		t.hint = "degree(expr, x)"
	case "coeff":
		t.hint = "coeff(expr, x, n)"
	case "collect":
		t.hint = "collect(expr, x)"
	case "factor":
		t.hint = "factor(expr[, x])"
	case "gcd":
		t.hint = "gcd(f, g[, x])"
	case "lcm":
		t.hint = "lcm(f, g[, x])"
	case "resultant":
		t.hint = "resultant(f, g, x)"
	case "newton":
		t.hint = "newton(expr, x0[, tol[, maxIter]])"
	case "bisection":
		t.hint = "bisection(expr, a, b[, tol[, maxIter]])"
	case "secant":
		t.hint = "secant(expr, x0, x1[, tol[, maxIter]])"
	case "diff_num":
		t.hint = "diff_num(expr, x[, h])"
	case "integrate_num":
		t.hint = "integrate_num(expr, a, b[, method[, n]])"
	case "interp":
		t.hint = "interp(data, x)"
	case "polyval":
		t.hint = "polyval(coeffs, x)"
	case "polyfit":
		t.hint = "polyfit(data, n) or polyfit(x, y, n)"
	case "solve":
		t.hint = "solve(A, b)"
	case "convolve":
		t.hint = "convolve(a, b)"
	case "cov":
		t.hint = "cov(x, y)"
	case "corr":
		t.hint = "corr(x, y)"
	case "hist":
		t.hint = "hist(data, bins)"
	case "implicit":
		t.hint = "implicit(expr, xmin, xmax, ymin, ymax[, n])"
	case "contour":
		t.hint = "contour(expr, levels, xmin, xmax, ymin, ymax[, n])"
	case "vectorfield":
		t.hint = "vectorfield(f, g, xmin, xmax, ymin, ymax[, n])"
	case "polar":
		t.hint = "polar(z)"
	case "rect":
		t.hint = "rect(r, phi)"
	case "solve1":
		t.hint = "solve1(expr, x0[, tol[, maxIter]])"
	case "solve2":
		t.hint = "solve2(f, g, x0, y0[, tol[, maxIter]])"
	case "roots":
		t.hint = "roots(expr, xmin, xmax[, n])"
	case "region":
		t.hint = "region(cond, xmin, xmax, ymin, ymax[, n])"
	case "plane":
		t.hint = "plane(n, d) or plane(p0, p1, p2)"
	case "param":
		t.hint = "param(x(t), y(t), tmin, tmax[, n])"
	case "expr":
		t.hint = "expr(x)"
	case "eval":
		t.hint = "eval(expr)"
	case "size":
		t.hint = "size(expr)"
	case "time":
		t.hint = "time(expr) (sets _time_ms)"
	case "numeric":
		t.hint = "numeric(expr)"
	case "exact":
		t.hint = "exact(expr)"
	case "if":
		t.hint = "if(cond, a, b)"
	case "where":
		t.hint = "where(cond, value)"
	case "and":
		t.hint = "and(a, b)"
	case "or":
		t.hint = "or(a, b)"
	case "not":
		t.hint = "not(a)"
	case "vec2":
		t.hint = "vec2(x, y)"
	case "vec3":
		t.hint = "vec3(x, y, z)"
	case "vec4":
		t.hint = "vec4(x, y, z, w)"
	case "dot":
		t.hint = "dot(a, b)"
	case "cross":
		t.hint = "cross(a, b)"
	case "mag", "norm":
		t.hint = "mag(v)"
	case "unit", "normalize":
		t.hint = "unit(v)"
	case "dist":
		t.hint = "dist(a, b)"
	case "angle":
		t.hint = "angle(a, b)"
	case "proj":
		t.hint = "proj(a, b)"
	case "outer":
		t.hint = "outer(a, b)"
	case "lerp":
		t.hint = "lerp(a, b, t)"
	case "get":
		t.hint = "get(v, i) or get(A, row, col)"
	case "set":
		t.hint = "set(v, i, value) or set(A, row, col, value)"
	case "qr":
		t.hint = "qr(A) (sets _Q, _R)"
	case "svd":
		t.hint = "svd(A) (returns s, sets _U, _V, _S)"
	default:
		if len(cands) > 1 {
			t.hint = fmt.Sprintf("Tab: complete (%d)", len(cands))
		}
	}
}

func (t *Task) updateCommandHint(line string) {
	_ = line
	start := 1
	for start < len(t.input) && t.input[start] == ' ' {
		start++
	}
	if start >= len(t.input) {
		cands := t.completeFromPrefix("", true)
		if len(cands) == 0 {
			return
		}
		best := pickBestCompletion("", cands)
		t.cands = cands
		t.best = best
		if t.cursor == len(t.input) && best != "" {
			t.ghost = best
		}
		return
	}
	end := start
	for end < len(t.input) && isIdentContinue(t.input[end]) {
		end++
	}
	if start >= end {
		return
	}
	prefixEnd := end
	if t.cursor >= start && t.cursor < end {
		prefixEnd = t.cursor
	}
	cmd := string(t.input[start:prefixEnd])
	switch cmd {
	case "help":
		t.hint = ":help"
	case "prec":
		t.hint = ":prec N"
	case "save":
		t.hint = ":save NAME|/path/file.vnb"
	case "load":
		t.hint = ":load NAME|/path/file.vnb"
	case "notebooks":
		t.hint = ":notebooks"
	case "new":
		t.hint = ":new"
	case "plotclear":
		t.hint = ":plotclear"
	case "plots":
		t.hint = ":plots"
	case "plotdel":
		t.hint = ":plotdel N"
	case "x", "xrange", "domain":
		t.hint = ":x A B"
	case "y", "yrange":
		t.hint = ":y A B"
	case "view":
		t.hint = ":view xmin xmax ymin ymax"
	case "exact":
		t.hint = ":exact"
	case "float":
		t.hint = ":float"
	case "clear":
		t.hint = ":clear (clear output)"
	case "term":
		t.hint = ":term"
	case "plot":
		t.hint = ":plot"
	case "stack":
		t.hint = ":stack"
	case "autoscale":
		t.hint = ":autoscale"
	case "resetview":
		t.hint = ":resetview"
	case "vars":
		t.hint = ":vars"
	case "funcs":
		t.hint = ":funcs"
	case "del":
		t.hint = ":del name"
	case "about":
		t.hint = ":about"
	}

	prefix := cmd
	cands := t.completeFromPrefix(prefix, true)
	if len(cands) == 0 {
		return
	}
	best := pickBestCompletion(prefix, cands)
	t.cands = cands
	t.best = best
	if best != prefix && t.cursor == prefixEnd && prefixEnd == len(t.input) {
		t.ghost = best[len(prefix):]
	}
	if len(cands) > 1 && t.hint == "" {
		t.hint = fmt.Sprintf("Tab: complete (%d)", len(cands))
	}
}

func (t *Task) updateServiceHint(line string) {
	_ = line
	start := 1
	for start < len(t.input) && t.input[start] == ' ' {
		start++
	}
	if start >= len(t.input) {
		cands := completeServiceFromPrefix("")
		if len(cands) == 0 {
			return
		}
		best := pickBestCompletion("", cands)
		t.cands = cands
		t.best = best
		if t.cursor == len(t.input) && best != "" {
			t.ghost = best
		}
		return
	}
	end := start
	for end < len(t.input) && isIdentContinue(t.input[end]) {
		end++
	}
	if start >= end {
		return
	}
	prefixEnd := end
	if t.cursor >= start && t.cursor < end {
		prefixEnd = t.cursor
	}
	cmd := string(t.input[start:prefixEnd])
	switch cmd {
	case "help":
		t.hint = "$help"
	case "about":
		t.hint = "$about"
	case "clear":
		t.hint = "$clear"
	case "resetview":
		t.hint = "$resetview"
	case "autoscale":
		t.hint = "$autoscale"
	case "vars":
		t.hint = "$vars"
	case "funcs":
		t.hint = "$funcs"
	case "del":
		t.hint = "$del name"
	}

	prefix := cmd
	cands := completeServiceFromPrefix(prefix)
	if len(cands) == 0 {
		return
	}
	best := pickBestCompletion(prefix, cands)
	t.cands = cands
	t.best = best
	if best != prefix && t.cursor == prefixEnd && prefixEnd == len(t.input) {
		t.ghost = best[len(prefix):]
	}
	if len(cands) > 1 && t.hint == "" {
		t.hint = fmt.Sprintf("Tab: complete (%d)", len(cands))
	}
}

func (t *Task) autocomplete() {
	if t.tab != tabTerminal || t.editVar != "" {
		return
	}

	isCmd := len(t.input) > 0 && t.input[0] == ':'
	if isCmd {
		t.autocompleteCommand()
		t.updateHint()
		return
	}
	isSvc := len(t.input) > 0 && t.input[0] == '$'
	if isSvc {
		t.autocompleteService()
		t.updateHint()
		return
	}

	start, end := identBoundsAt(t.input, t.cursor)
	if start == end || t.cursor < start {
		return
	}
	prefix := string(t.input[start:t.cursor])
	if prefix == "" {
		return
	}
	cands := t.completeFromPrefix(prefix, false)
	t.applyCompletion(start, end, prefix, cands)
	t.updateHint()
}

func (t *Task) autocompleteCommand() {
	start := 1
	for start < len(t.input) && t.input[start] == ' ' {
		start++
	}
	end := start
	for end < len(t.input) && isIdentContinue(t.input[end]) {
		end++
	}
	if start >= end {
		return
	}
	prefixEnd := end
	if t.cursor >= start && t.cursor < end {
		prefixEnd = t.cursor
	}
	prefix := string(t.input[start:prefixEnd])
	if prefix == "" {
		return
	}
	cands := t.completeFromPrefix(prefix, true)
	t.applyCompletion(start, end, prefix, cands)
}

func (t *Task) autocompleteService() {
	start := 1
	for start < len(t.input) && t.input[start] == ' ' {
		start++
	}
	end := start
	for end < len(t.input) && isIdentContinue(t.input[end]) {
		end++
	}
	if start >= end {
		return
	}
	prefixEnd := end
	if t.cursor >= start && t.cursor < end {
		prefixEnd = t.cursor
	}
	prefix := string(t.input[start:prefixEnd])
	if prefix == "" {
		return
	}
	cands := completeServiceFromPrefix(prefix)
	t.applyCompletion(start, end, prefix, cands)
}

func (t *Task) applyCompletion(start, end int, prefix string, cands []string) {
	if len(cands) == 0 {
		t.setHint("no completions")
		return
	}

	common := commonPrefix(cands)
	if common != "" && common != prefix {
		t.replaceInputRange(start, end, common)
		return
	}

	if len(cands) == 1 {
		t.replaceInputRange(start, end, cands[0])
		return
	}
}

func (t *Task) replaceInputRange(start, end int, replacement string) {
	if start < 0 || end < start || start > len(t.input) || end > len(t.input) {
		return
	}
	before := append([]rune(nil), t.input[:start]...)
	after := append([]rune(nil), t.input[end:]...)
	t.input = append(before, append([]rune(replacement), after...)...)
	t.cursor = start + len([]rune(replacement))
}

func (t *Task) completeFromPrefix(prefix string, commands bool) []string {
	var cands []string
	add := func(s string) {
		if strings.HasPrefix(s, prefix) {
			cands = append(cands, s)
		}
	}

	if commands {
		for _, s := range []string{
			"help",
			"save", "load", "notebooks", "new",
			"exact", "float", "prec",
			"plotclear", "plots", "plotdel",
			"x", "xrange", "domain",
			"y", "yrange",
			"view",
			"autoscale", "resetview",
			"vars", "funcs", "del", "about",
			"clear", "term", "plot", "stack",
		} {
			add(s)
		}
		sort.Strings(cands)
		return cands
	}

	for _, s := range builtinKeywords() {
		add(s)
	}
	for name := range t.e.funcs {
		add(name)
	}
	for name := range t.e.vars {
		add(name)
	}
	sort.Strings(cands)
	return cands
}

func commonPrefix(ss []string) string {
	if len(ss) == 0 {
		return ""
	}
	p := ss[0]
	for _, s := range ss[1:] {
		for !strings.HasPrefix(s, p) {
			if p == "" {
				return ""
			}
			p = p[:len(p)-1]
		}
	}
	return p
}

func identBoundsAt(rs []rune, cursor int) (start, end int) {
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(rs) {
		cursor = len(rs)
	}
	start = cursor
	for start > 0 && isIdentContinue(rs[start-1]) {
		start--
	}
	end = cursor
	for end < len(rs) && isIdentContinue(rs[end]) {
		end++
	}
	return start, end
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
	case valueComplex:
		re := real(v.c)
		im := imag(v.c)
		if im == 0 {
			return formatFloat(re, t.e.prec)
		}
		if re == 0 {
			return formatFloat(im, t.e.prec) + "*i"
		}
		sign := "+"
		if im < 0 {
			sign = "-"
			im = -im
		}
		return fmt.Sprintf("%s%s%s*i", formatFloat(re, t.e.prec), sign, formatFloat(im, t.e.prec))
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

func (t *Task) normalizeView() {
	t.xMin = normalizeFloat(t.xMin)
	t.xMax = normalizeFloat(t.xMax)
	t.yMin = normalizeFloat(t.yMin)
	t.yMax = normalizeFloat(t.yMax)
	if t.xMin == 0 {
		t.xMin = 0
	}
	if t.xMax == 0 {
		t.xMax = 0
	}
	if t.yMin == 0 {
		t.yMin = 0
	}
	if t.yMax == 0 {
		t.yMax = 0
	}
}

func normalizeFloat(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return v
	}
	s := strconv.FormatFloat(v, 'g', 12, 64)
	out, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return v
	}
	return out
}

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
