package gpioscope

import (
	"fmt"
	"sort"

	"spark/hal"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"

	"tinygo.org/x/tinyfont"
)

type mode uint8

const (
	modeGPIO mode = iota
	modeSignal
	modeProtocol
)

type triggerKind uint8

const (
	trigNone triggerKind = iota
	trigRise
	trigFall
	trigHigh
	trigLow
)

type protoKind uint8

const (
	protoNone protoKind = iota
	protoUART
	protoSPI
	protoI2C
)

type pin struct {
	id    uint8
	caps  proto.GPIOPinCaps
	mode  proto.GPIOMode
	pull  proto.GPIOPull
	level bool

	selected bool
}

type ring struct {
	buf   []uint32
	head  int
	count int
}

func newRing(n int) *ring {
	if n < 1 {
		n = 1
	}
	return &ring{buf: make([]uint32, n)}
}

func (r *ring) Len() int { return r.count }

func (r *ring) Append(v uint32) {
	if len(r.buf) == 0 {
		return
	}
	r.buf[r.head] = v
	r.head++
	if r.head >= len(r.buf) {
		r.head = 0
	}
	if r.count < len(r.buf) {
		r.count++
	}
}

func (r *ring) At(i int) uint32 {
	if i < 0 || i >= r.count || len(r.buf) == 0 {
		return 0
	}
	start := r.head - r.count
	if start < 0 {
		start += len(r.buf)
	}
	idx := start + i
	if idx >= len(r.buf) {
		idx -= len(r.buf)
	}
	return r.buf[idx]
}

func (r *ring) LastN(n int) []uint32 {
	if n <= 0 || r.count == 0 {
		return nil
	}
	if n > r.count {
		n = r.count
	}
	out := make([]uint32, n)
	start := r.count - n
	for i := 0; i < n; i++ {
		out[i] = r.At(start + i)
	}
	return out
}

// Task implements GPIO Control / Signal Viewer / Protocol View.
type Task struct {
	disp hal.Display
	ep   kernel.Capability

	timeCap kernel.Capability
	gpioCap kernel.Capability

	fb hal.Framebuffer
	d  *fbDisplay

	font       tinyfont.Fonter
	fontWidth  int16
	fontHeight int16
	fontOffset int16

	cols int
	rows int

	active bool
	muxCap kernel.Capability

	inbuf []byte

	mode mode

	pins []pin
	sel  int

	msg string

	replyEP     kernel.Capability
	replySend   kernel.Capability
	replyRecv   kernel.Capability
	gpioCh      <-chan kernel.Message
	nextReqID   uint32
	listReqID   uint32
	listPending bool

	sleepEP          kernel.Capability
	sleepSendCap     kernel.Capability
	sleepRecvCap     kernel.Capability
	sleepCh          <-chan kernel.Message
	sampleSleepReqID uint32
	running          bool
	periodTicks      uint32

	pulseSleepReqID uint32
	pulsePinID      uint8
	pulsePending    bool
	pulseTicks      uint32

	buf          *ring
	frozen       []uint32
	frozenActive bool

	samplesPerPx int
	scroll       int
	cursor       int

	trigger      triggerKind
	triggerPinID uint8
	triggerArmed bool
	triggered    bool
	postRemain   int
	preSamples   int
	postSamples  int
	lastTrigLvl  bool
	singleShot   bool

	pk protoKind

	uartRX   int
	uartBaud int

	spiCLK  int
	spiMOSI int
	spiMISO int
	spiCS   int
	spiCPOL bool
	spiCPHA bool

	i2cSCL int
	i2cSDA int

	decoded []string
}

func New(disp hal.Display, ep, timeCap, gpioCap kernel.Capability) *Task {
	return &Task{
		disp:         disp,
		ep:           ep,
		timeCap:      timeCap,
		gpioCap:      gpioCap,
		mode:         modeGPIO,
		periodTicks:  1,
		pulseTicks:   10,
		buf:          newRing(4096),
		samplesPerPx: 2,
		cursor:       -1,
		preSamples:   250,
		postSamples:  250,
		uartRX:       -1,
		uartBaud:     300,
		spiCLK:       -1,
		spiMOSI:      -1,
		spiMISO:      -1,
		spiCS:        -1,
		i2cSCL:       -1,
		i2cSDA:       -1,
	}
}

func (t *Task) Run(ctx *kernel.Context) {
	appCh, ok := ctx.RecvChan(t.ep)
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

	font, fw, fh, fo, ok := initFont()
	if !ok {
		return
	}
	t.font = font
	t.fontWidth = fw
	t.fontHeight = fh
	t.fontOffset = fo

	t.cols = t.fb.Width() / int(t.fontWidth)
	t.rows = t.fb.Height() / int(t.fontHeight)
	if t.cols <= 0 || t.rows <= 0 {
		return
	}

	t.initReplyEndpoints(ctx)
	t.reloadPins(ctx)
	t.render()

	for {
		select {
		case msg, ok := <-appCh:
			if !ok {
				return
			}
			t.handleAppMsg(ctx, msg)

		case msg := <-t.gpioCh:
			t.handleGPIOReply(ctx, msg)

		case msg := <-t.sleepCh:
			t.handleWake(ctx, msg)
		}
	}
}

func (t *Task) initReplyEndpoints(ctx *kernel.Context) {
	t.replyEP = ctx.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	t.replySend = t.replyEP.Restrict(kernel.RightSend)
	t.replyRecv = t.replyEP.Restrict(kernel.RightRecv)
	t.gpioCh, _ = ctx.RecvChan(t.replyRecv)

	t.sleepEP = ctx.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	t.sleepSendCap = t.sleepEP.Restrict(kernel.RightSend)
	t.sleepRecvCap = t.sleepEP.Restrict(kernel.RightRecv)
	t.sleepCh, _ = ctx.RecvChan(t.sleepRecvCap)
}

func (t *Task) handleAppMsg(ctx *kernel.Context, msg kernel.Message) {
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
			return
		}
		t.setActive(active)
		if t.active {
			t.render()
		}

	case proto.MsgAppSelect:
		appID, _, ok := proto.DecodeAppSelectPayload(msg.Data[:msg.Len])
		if !ok || appID != proto.AppGPIOScope {
			return
		}
		t.reloadPins(ctx)
		if t.active {
			t.render()
		}

	case proto.MsgTermInput:
		if !t.active {
			return
		}
		t.handleInput(ctx, msg.Data[:msg.Len])
		if t.active {
			t.render()
		}
	}
}

func (t *Task) setActive(active bool) {
	if active == t.active {
		if !active {
			t.unload()
		}
		return
	}
	t.active = active
	if !t.active {
		t.unload()
		return
	}
	t.msg = "Tab mode | ↑↓ select | Space watch | r run | q quit"
}

func (t *Task) unload() {
	t.active = false
	t.running = false
	t.triggerArmed = false
	t.triggered = false
	t.frozenActive = false
	t.decoded = nil
}

func (t *Task) requestExit(ctx *kernel.Context) {
	if t.fb != nil {
		t.fb.ClearRGB(0, 0, 0)
		_ = t.fb.Present()
	}
	t.unload()

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

func (t *Task) nextID() uint32 {
	t.nextReqID++
	if t.nextReqID == 0 {
		t.nextReqID++
	}
	return t.nextReqID
}

func (t *Task) reloadPins(ctx *kernel.Context) {
	t.pins = nil
	t.sel = 0
	t.listReqID = t.nextID()
	t.listPending = true
	t.msg = "loading GPIO..."
	t.sendGPIO(ctx, proto.MsgGPIOList, proto.GPIOListPayload(t.listReqID), t.replySend)
}

func (t *Task) sendGPIO(ctx *kernel.Context, kind proto.Kind, payload []byte, reply kernel.Capability) {
	if !t.gpioCap.Valid() {
		t.msg = "gpio: no capability"
		return
	}
	for {
		res := ctx.SendToCapResult(t.gpioCap, uint16(kind), payload, reply)
		switch res {
		case kernel.SendOK:
			return
		case kernel.SendErrQueueFull:
			ctx.BlockOnTick()
		default:
			t.msg = fmt.Sprintf("gpio send: %s", res)
			return
		}
	}
}

func (t *Task) handleGPIOReply(ctx *kernel.Context, msg kernel.Message) {
	switch proto.Kind(msg.Kind) {
	case proto.MsgGPIOListResp:
		reqID, done, pinID, caps, mode, pull, level, ok := proto.DecodeGPIOListRespPayload(msg.Data[:msg.Len])
		if !ok || !t.listPending || reqID != t.listReqID {
			return
		}
		if done {
			t.listPending = false
			sort.Slice(t.pins, func(i, j int) bool { return t.pins[i].id < t.pins[j].id })
			if t.sel >= len(t.pins) {
				t.sel = len(t.pins) - 1
			}
			if t.sel < 0 {
				t.sel = 0
			}
			t.msg = "Tab mode | ↑↓ select | Space watch | r run | q quit"
			return
		}
		t.pins = append(t.pins, pin{id: pinID, caps: caps, mode: mode, pull: pull, level: level})

	case proto.MsgGPIOConfigResp:
		_, pinID, mode, pull, level, ok := proto.DecodeGPIOConfigRespPayload(msg.Data[:msg.Len])
		if !ok {
			return
		}
		t.updatePinState(pinID, mode, pull, level)

	case proto.MsgGPIOWriteResp:
		_, pinID, level, ok := proto.DecodeGPIOWriteRespPayload(msg.Data[:msg.Len])
		if !ok {
			return
		}
		t.updatePinLevel(pinID, level)

	case proto.MsgGPIOReadResp:
		reqID, mask, levels, ok := proto.DecodeGPIOReadRespPayload(msg.Data[:msg.Len])
		if !ok {
			_ = reqID
			return
		}
		_ = mask
		t.onSample(levels)

	case proto.MsgError:
		code, ref, detail, ok := proto.DecodeErrorPayload(msg.Data[:msg.Len])
		if !ok {
			return
		}
		if reqID, rest, ok := proto.DecodeErrorDetailWithRequestID(detail); ok {
			_ = reqID
			detail = rest
		}
		t.msg = fmt.Sprintf("gpio error: code=%s ref=%s %s", code, ref, string(detail))
	}

	_ = ctx
}

func (t *Task) updatePinState(pinID uint8, mode proto.GPIOMode, pull proto.GPIOPull, level bool) {
	for i := range t.pins {
		if t.pins[i].id != pinID {
			continue
		}
		t.pins[i].mode = mode
		t.pins[i].pull = pull
		t.pins[i].level = level
		return
	}
}

func (t *Task) updatePinLevel(pinID uint8, level bool) {
	for i := range t.pins {
		if t.pins[i].id != pinID {
			continue
		}
		t.pins[i].level = level
		return
	}
}

func (t *Task) handleWake(ctx *kernel.Context, msg kernel.Message) {
	switch proto.Kind(msg.Kind) {
	case proto.MsgWake:
	case proto.MsgError:
		code, ref, detail, ok := proto.DecodeErrorPayload(msg.Data[:msg.Len])
		if !ok {
			return
		}
		if reqID, rest, ok := proto.DecodeErrorDetailWithRequestID(detail); ok {
			_ = reqID
			detail = rest
		}
		t.msg = fmt.Sprintf("time error: code=%s ref=%s %s", code, ref, string(detail))
		return
	default:
		return
	}

	reqID, ok := proto.DecodeWakePayload(msg.Data[:msg.Len])
	if !ok {
		return
	}

	if reqID == t.sampleSleepReqID {
		if t.running && t.active {
			t.sendRead(ctx)
			t.scheduleWake(ctx)
		}
		return
	}
	if reqID == t.pulseSleepReqID && t.pulsePending {
		t.pulsePending = false
		t.sendGPIO(ctx, proto.MsgGPIOWrite, proto.GPIOWritePayload(t.nextID(), t.pulsePinID, false), t.replySend)
		return
	}
}

func (t *Task) scheduleWake(ctx *kernel.Context) {
	if !t.timeCap.Valid() || !t.sleepSendCap.Valid() {
		return
	}
	t.sampleSleepReqID = t.nextID()
	payload := proto.SleepPayload(t.sampleSleepReqID, t.periodTicks)
	for {
		res := ctx.SendToCapResult(t.timeCap, uint16(proto.MsgSleep), payload, t.sleepSendCap)
		switch res {
		case kernel.SendOK:
			return
		case kernel.SendErrQueueFull:
			ctx.BlockOnTick()
		default:
			t.msg = fmt.Sprintf("time send: %s", res)
			return
		}
	}
}

func (t *Task) selectedMask() uint32 {
	var mask uint32
	for _, p := range t.pins {
		if p.selected && p.id < 32 {
			mask |= 1 << p.id
		}
	}
	if t.triggerArmed && t.triggerPinID < 32 {
		mask |= 1 << t.triggerPinID
	}
	return mask
}

func (t *Task) sendRead(ctx *kernel.Context) {
	mask := t.selectedMask()
	if mask == 0 {
		return
	}
	reqID := t.nextID()
	t.sendGPIO(ctx, proto.MsgGPIORead, proto.GPIOReadPayload(reqID, mask), t.replySend)
}

func (t *Task) onSample(levels uint32) {
	t.buf.Append(levels)

	if t.triggerArmed && !t.triggered {
		lvl := levels&(1<<t.triggerPinID) != 0
		if t.matchTrigger(t.lastTrigLvl, lvl) {
			t.triggered = true
			t.postRemain = t.postSamples
		}
		t.lastTrigLvl = lvl
	}

	if t.triggered {
		t.postRemain--
		if t.postRemain <= 0 {
			t.frozen = t.buf.LastN(t.preSamples + t.postSamples)
			t.frozenActive = true
			if t.singleShot {
				t.running = false
				t.triggerArmed = false
				t.triggered = false
				return
			}
			t.triggered = false
		}
	}
}

func (t *Task) matchTrigger(prev, cur bool) bool {
	switch t.trigger {
	case trigRise:
		return !prev && cur
	case trigFall:
		return prev && !cur
	case trigHigh:
		return cur
	case trigLow:
		return !cur
	default:
		return false
	}
}

func (t *Task) handleInput(ctx *kernel.Context, b []byte) {
	t.inbuf = append(t.inbuf, b...)

	for len(t.inbuf) > 0 {
		n, k, ok := nextKey(t.inbuf)
		if !ok {
			return
		}
		if n == 0 {
			return
		}
		t.inbuf = t.inbuf[n:]
		if t.handleKey(ctx, k) {
			return
		}
	}
}

func (t *Task) handleKey(ctx *kernel.Context, k key) (exit bool) {
	switch k.kind {
	case keyEsc:
		if t.running {
			t.running = false
			t.msg = "stopped"
			return false
		}
		t.requestExit(ctx)
		return true

	case keyRune:
		switch k.r {
		case 'q':
			t.requestExit(ctx)
			return true
		case 'r':
			t.running = !t.running
			t.frozenActive = false
			t.decoded = nil
			t.triggered = false
			if t.running {
				t.msg = "running"
				t.scheduleWake(ctx)
			} else {
				t.msg = "stopped"
			}
		case ' ':
			t.toggleSelected()
		case 't':
			t.cycleTrigger()
		case 's':
			t.singleShot = !t.singleShot
		case 'i':
			t.setMode(ctx, proto.GPIOModeInput)
		case 'o':
			t.setMode(ctx, proto.GPIOModeOutput)
		case 'u':
			t.setPull(ctx, proto.GPIOPullUp)
		case 'd':
			if t.mode == modeProtocol {
				t.decode()
				return false
			}
			t.setPull(ctx, proto.GPIOPullDown)
		case 'n':
			t.setPull(ctx, proto.GPIOPullNone)
		case 'h':
			t.writeLevel(ctx, true)
		case 'l':
			t.writeLevel(ctx, false)
		case 'x':
			t.toggleLevel(ctx)
		case 'p':
			t.pulse(ctx)
		case ',':
			if t.periodTicks > 1 {
				t.periodTicks--
			}
		case '.':
			if t.periodTicks < 1000 {
				t.periodTicks++
			}
		case '+':
			t.samplesPerPx = clampInt(t.samplesPerPx-1, 1, 64)
		case '-':
			t.samplesPerPx = clampInt(t.samplesPerPx+1, 1, 64)
		case 'c':
			t.toggleCursor()
		case '[':
			t.moveCursor(-t.samplesPerPx)
		case ']':
			t.moveCursor(t.samplesPerPx)
		case '1':
			t.pk = protoNone
		case '2':
			t.pk = protoUART
		case '3':
			t.pk = protoSPI
		case '4':
			t.pk = protoI2C
		case 'R':
			t.assignUARTRX()
		case 'C':
			t.assignSPI("clk")
		case 'M':
			t.assignSPI("mosi")
		case 'I':
			t.assignSPI("miso")
		case 'S':
			t.assignSPI("cs")
		case 'P':
			t.spiCPOL = !t.spiCPOL
		case 'H':
			t.spiCPHA = !t.spiCPHA
		case 'A':
			t.assignI2C("scl")
		case 'D':
			t.assignI2C("sda")
		}

	case keyTab:
		t.mode++
		if t.mode > modeProtocol {
			t.mode = modeGPIO
		}

	case keyUp:
		if t.sel > 0 {
			t.sel--
		}

	case keyDown:
		if t.sel+1 < len(t.pins) {
			t.sel++
		}

	case keyLeft:
		if t.scroll < 1_000_000 {
			t.scroll += t.samplesPerPx
		}

	case keyRight:
		t.scroll -= t.samplesPerPx
		if t.scroll < 0 {
			t.scroll = 0
		}

	case keyPageUp:
		t.scroll += 200 * t.samplesPerPx

	case keyPageDown:
		t.scroll -= 200 * t.samplesPerPx
		if t.scroll < 0 {
			t.scroll = 0
		}
	}

	_ = ctx
	return false
}

func (t *Task) toggleSelected() {
	if t.sel < 0 || t.sel >= len(t.pins) {
		return
	}
	t.pins[t.sel].selected = !t.pins[t.sel].selected
}

func (t *Task) cycleTrigger() {
	if t.sel < 0 || t.sel >= len(t.pins) {
		return
	}
	t.triggerArmed = true
	t.triggerPinID = t.pins[t.sel].id
	t.trigger++
	if t.trigger > trigLow {
		t.trigger = trigNone
		t.triggerArmed = false
	}
	t.triggered = false
	t.postRemain = 0
	t.lastTrigLvl = false
}

func (t *Task) setMode(ctx *kernel.Context, mode proto.GPIOMode) {
	if t.sel < 0 || t.sel >= len(t.pins) {
		return
	}
	p := t.pins[t.sel]
	reqID := t.nextID()
	t.sendGPIO(ctx, proto.MsgGPIOConfig, proto.GPIOConfigPayload(reqID, p.id, mode, p.pull), t.replySend)
}

func (t *Task) setPull(ctx *kernel.Context, pull proto.GPIOPull) {
	if t.sel < 0 || t.sel >= len(t.pins) {
		return
	}
	p := t.pins[t.sel]
	reqID := t.nextID()
	t.sendGPIO(ctx, proto.MsgGPIOConfig, proto.GPIOConfigPayload(reqID, p.id, p.mode, pull), t.replySend)
}

func (t *Task) writeLevel(ctx *kernel.Context, level bool) {
	if t.sel < 0 || t.sel >= len(t.pins) {
		return
	}
	p := t.pins[t.sel]
	reqID := t.nextID()
	t.sendGPIO(ctx, proto.MsgGPIOWrite, proto.GPIOWritePayload(reqID, p.id, level), t.replySend)
}

func (t *Task) toggleLevel(ctx *kernel.Context) {
	if t.sel < 0 || t.sel >= len(t.pins) {
		return
	}
	p := t.pins[t.sel]
	t.writeLevel(ctx, !p.level)
}

func (t *Task) pulse(ctx *kernel.Context) {
	if t.sel < 0 || t.sel >= len(t.pins) {
		return
	}
	if !t.timeCap.Valid() {
		t.msg = "pulse: no time capability"
		return
	}
	p := t.pins[t.sel]
	t.pulsePinID = p.id
	t.pulsePending = true
	t.sendGPIO(ctx, proto.MsgGPIOWrite, proto.GPIOWritePayload(t.nextID(), p.id, true), t.replySend)

	t.pulseSleepReqID = t.nextID()
	payload := proto.SleepPayload(t.pulseSleepReqID, t.pulseTicks)
	for {
		res := ctx.SendToCapResult(t.timeCap, uint16(proto.MsgSleep), payload, t.sleepSendCap)
		switch res {
		case kernel.SendOK:
			return
		case kernel.SendErrQueueFull:
			ctx.BlockOnTick()
		default:
			t.msg = fmt.Sprintf("time send: %s", res)
			t.pulsePending = false
			return
		}
	}
}

func (t *Task) toggleCursor() {
	if t.cursor >= 0 {
		t.cursor = -1
		return
	}
	t.cursor = 0
}

func (t *Task) moveCursor(delta int) {
	if t.cursor < 0 {
		return
	}
	t.cursor += delta
	if t.cursor < 0 {
		t.cursor = 0
	}
}

func (t *Task) assignUARTRX() {
	if t.sel < 0 || t.sel >= len(t.pins) {
		return
	}
	t.uartRX = int(t.pins[t.sel].id)
}

func (t *Task) assignSPI(which string) {
	if t.sel < 0 || t.sel >= len(t.pins) {
		return
	}
	id := int(t.pins[t.sel].id)
	switch which {
	case "clk":
		t.spiCLK = id
	case "mosi":
		t.spiMOSI = id
	case "miso":
		t.spiMISO = id
	case "cs":
		t.spiCS = id
	}
}

func (t *Task) assignI2C(which string) {
	if t.sel < 0 || t.sel >= len(t.pins) {
		return
	}
	id := int(t.pins[t.sel].id)
	switch which {
	case "scl":
		t.i2cSCL = id
	case "sda":
		t.i2cSDA = id
	}
}

func (t *Task) decode() {
	samples := t.samplesForDecode()
	if len(samples) == 0 {
		t.msg = "decode: no samples"
		return
	}

	var out []string
	switch t.pk {
	case protoUART:
		out = decodeUART(samples, t.uartRX, t.uartBaud, t.periodTicks)
	case protoSPI:
		out = decodeSPI(samples, t.spiCLK, t.spiMOSI, t.spiMISO, t.spiCS, t.spiCPOL, t.spiCPHA)
	case protoI2C:
		out = decodeI2C(samples, t.i2cSCL, t.i2cSDA)
	default:
		out = []string{"protocol: none (2 UART, 3 SPI, 4 I2C)"}
	}
	t.decoded = joinTrimLines(out, 12)
}

func (t *Task) samplesForDecode() []uint32 {
	if t.frozenActive {
		return append([]uint32(nil), t.frozen...)
	}
	if t.buf.Len() == 0 {
		return nil
	}
	return t.buf.LastN(2048)
}

func (t *Task) render() {
	if t.fb == nil || t.d == nil {
		return
	}
	t.d.FillRectangle(0, 0, int16(t.fb.Width()), int16(t.fb.Height()), colorBG)

	leftPx := int16(18) * t.fontWidth
	if leftPx < t.fontWidth*12 {
		leftPx = t.fontWidth * 12
	}
	if leftPx > int16(t.fb.Width())/2 {
		leftPx = int16(t.fb.Width()) / 2
	}

	headerH := t.fontHeight + 2
	footerH := t.fontHeight + 2

	t.d.FillRectangle(0, 0, int16(t.fb.Width()), headerH, colorHeaderBG)
	t.d.FillRectangle(0, int16(t.fb.Height())-footerH, int16(t.fb.Width()), footerH, colorHeaderBG)
	t.d.FillRectangle(0, headerH, leftPx, int16(t.fb.Height())-headerH-footerH, colorPanelBG)

	writeText(t.d, t.font, 2, t.fontOffset+1, colorFG, "GPIO / Signal / Protocol Viewer")

	modeName := "GPIO"
	switch t.mode {
	case modeSignal:
		modeName = "Signal"
	case modeProtocol:
		modeName = "Protocol"
	}
	runName := "stop"
	if t.running {
		runName = "run"
	}
	trig := "trig:none"
	if t.triggerArmed {
		trig = fmt.Sprintf("trig:%s@%d", t.triggerName(), t.triggerPinID)
	}
	footer := fmt.Sprintf("%s | %s | %s | zoom:%d | %s", modeName, runName, fmtHz(t.periodTicks), t.samplesPerPx, trig)
	writeText(t.d, t.font, 2, int16(t.fb.Height())-footerH+t.fontOffset, colorFG, fitText(footer, t.cols))

	if t.msg != "" {
		writeText(t.d, t.font, 2, int16(t.fb.Height())-footerH-t.fontHeight+t.fontOffset, colorDim, fitText(t.msg, t.cols))
	}

	t.renderPins(headerH, leftPx, footerH)
	t.renderWave(headerH, leftPx, footerH)

	_ = t.fb.Present()
}

func (t *Task) triggerName() string {
	switch t.trigger {
	case trigRise:
		return "rise"
	case trigFall:
		return "fall"
	case trigHigh:
		return "high"
	case trigLow:
		return "low"
	default:
		return "none"
	}
}

func (t *Task) renderPins(headerH, leftPx, footerH int16) {
	_ = footerH
	x0 := int16(0)
	y0 := headerH + 2
	rowY := y0

	writeText(t.d, t.font, 2, rowY+t.fontOffset, colorDim, "Pins (Space=watch)")
	rowY += t.fontHeight

	maxRows := int((int16(t.fb.Height()) - footerH - rowY - 2) / t.fontHeight)
	if maxRows < 1 {
		return
	}
	top := 0
	if t.sel >= maxRows {
		top = t.sel - maxRows + 1
	}
	for i := 0; i < maxRows && top+i < len(t.pins); i++ {
		p := t.pins[top+i]
		y := rowY + int16(i)*t.fontHeight
		bg := colorPanelBG
		fg := colorFG
		if top+i == t.sel {
			bg = colorSelBG
			fg = colorSelFG
			t.d.FillRectangle(x0, y-1, leftPx, t.fontHeight+1, bg)
		}
		sel := ' '
		if p.selected {
			sel = '*'
		}
		m := "in"
		if p.mode == proto.GPIOModeOutput {
			m = "out"
		}
		l := '0'
		if p.level {
			l = '1'
		}
		line := fmt.Sprintf("%c %02d %-3s %c", sel, p.id, m, l)
		writeText(t.d, t.font, 2, y+t.fontOffset, fg, fitText(line, int(leftPx/t.fontWidth)-1))
	}
}

func (t *Task) renderWave(headerH, leftPx, footerH int16) {
	x0 := leftPx + 2
	y0 := headerH + 2
	w := int16(t.fb.Width()) - x0 - 2
	h := int16(t.fb.Height()) - y0 - footerH - 2
	if w <= 0 || h <= 0 {
		return
	}

	var watch []pin
	for _, p := range t.pins {
		if p.selected {
			watch = append(watch, p)
		}
	}
	if len(watch) == 0 {
		writeText(t.d, t.font, x0, y0+t.fontOffset, colorDim, "No watched pins (Space to toggle).")
		return
	}

	var samples []uint32
	if t.frozenActive {
		samples = t.frozen
	} else {
		samples = nil
	}
	total := 0
	if t.frozenActive {
		total = len(samples)
	} else {
		total = t.buf.Len()
	}
	if total == 0 {
		writeText(t.d, t.font, x0, y0+t.fontOffset, colorDim, "No samples yet (r to run).")
		return
	}

	spp := t.samplesPerPx
	if spp < 1 {
		spp = 1
	}
	visible := int(w) * spp
	end := total - t.scroll
	if end < 0 {
		end = 0
	}
	start := end - visible
	if start < 0 {
		start = 0
	}

	laneH := int(h) / len(watch)
	if laneH < 10 {
		laneH = 10
	}

	for li, p := range watch {
		lY := int(y0) + li*laneH
		hiY := int16(lY + 2)
		loY := int16(lY + laneH - 3)
		t.d.FillRectangle(x0, int16(lY), w, int16(laneH), colorBG)

		var last bool
		for xi := 0; xi < int(w); xi++ {
			si := start + xi*spp
			if si >= end {
				break
			}
			level := t.sampleAt(samples, si, p.id)
			y := loY
			if level {
				y = hiY
			}
			if xi == 0 {
				last = level
			}
			if level != last {
				t.d.FillRectangle(x0+int16(xi), hiY, 1, loY-hiY+1, colorDim)
			}
			last = level
			c := colorWaveLo
			if level {
				c = colorWaveHi
			}
			t.d.FillRectangle(x0+int16(xi), y, 1, 2, c)
		}

		label := fmt.Sprintf("%02d", p.id)
		writeText(t.d, t.font, x0, int16(lY)+t.fontOffset, colorDim, label)
	}

	if t.cursor >= 0 {
		cursor := t.cursor
		vis := end - start
		if vis < 0 {
			vis = 0
		}
		if cursor >= vis {
			cursor = vis - 1
		}
		if cursor >= 0 {
			cx := x0 + int16(cursor/spp)
			if cx >= x0 && cx < x0+w {
				t.d.FillRectangle(cx, y0, 1, h, colorCursor)
			}
		}
	}

	if t.mode == modeProtocol && len(t.decoded) > 0 {
		lines := joinTrimLines(t.decoded, 12)
		blockH := int16(len(lines))*t.fontHeight + 2
		if blockH < h {
			ty := y0 + h - blockH
			t.d.FillRectangle(x0, ty, w, blockH, colorHeaderBG)
			for i, s := range lines {
				writeText(t.d, t.font, x0+2, ty+int16(i)*t.fontHeight+t.fontOffset, colorFG, fitText(s, t.cols))
			}
		}
	}
}

func (t *Task) sampleAt(frozen []uint32, i int, pinID uint8) bool {
	var v uint32
	if t.frozenActive {
		if i < 0 || i >= len(frozen) {
			return false
		}
		v = frozen[i]
	} else {
		v = t.buf.At(i)
	}
	return v&(1<<pinID) != 0
}
