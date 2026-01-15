package rfanalyzer

import (
	"spark/hal"
	vfsclient "spark/sparkos/client/vfs"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

type focusPanel uint8

const (
	focusSpectrum focusPanel = iota
	focusWaterfall
	focusRFControl
	focusSniffer
	focusProtocol
)

type Task struct {
	disp hal.Display
	ep   kernel.Capability

	vfsCap kernel.Capability
	vfs    *vfsclient.Client

	fb hal.Framebuffer
	d  *fbDisplay

	cols     int
	rows     int
	mainRows int

	font       fonter
	fontWidth  int16
	fontHeight int16
	fontOffset int16

	active bool
	muxCap kernel.Capability

	focus focusPanel

	inbuf []byte

	nowTick uint64

	scanActive      bool
	waterfallFrozen bool
	capturePaused   bool
	selectedChannel int
	channelRangeLo  int
	channelRangeHi  int
	dwellTimeMs     int
	scanSpeedScalar int
	dataRate        rfDataRate
	crcMode         rfCRCMode
	autoAck         bool
	powerLevel      rfPowerLevel
	selectedSetting int

	scanChan      int
	scanNextTick  uint64
	sweepCount    uint64
	lastSweepTick uint64

	energyCur  [numChannels]uint8
	energyAvg  [numChannels]uint8
	energyPeak [numChannels]uint8

	wfPalette    wfPalette
	wfPalette565 [256]uint16
	wfW          int
	wfH          int
	wfHead       int
	wfBuf        []uint8

	activePreset string
	presetDirty  bool

	rng uint32

	nextRenderTick uint64

	showMenu    bool
	showHelp    bool
	showFilters bool
	dirty       dirtyFlags
}

func New(disp hal.Display, ep, vfsCap kernel.Capability) *Task {
	return &Task{
		disp:            disp,
		ep:              ep,
		vfsCap:          vfsCap,
		focus:           focusSpectrum,
		selectedChannel: 37,
		channelRangeLo:  0,
		channelRangeHi:  maxChannel,
		dwellTimeMs:     5,
		scanSpeedScalar: 1,
		dataRate:        rfRate2M,
		crcMode:         rfCRC2B,
		autoAck:         false,
		powerLevel:      rfPwrMax,
		wfPalette:       wfPaletteCyan,
		rng:             0xA341316C,
		dirty:           dirtyAll,
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
	t.mainRows = t.rows - headerRows - statusRows
	if t.cols <= 0 || t.rows <= 0 || t.mainRows <= 0 {
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
				t.setActive(ctx, active)

			case proto.MsgAppSelect:
				appID, _, ok := proto.DecodeAppSelectPayload(msg.Data[:msg.Len])
				if !ok || appID != proto.AppRFAnalyzer {
					continue
				}
				if t.active {
					t.invalidate(dirtyAll)
					t.renderNow(ctx.NowTick())
				}

			case proto.MsgTermInput:
				if !t.active {
					continue
				}
				t.handleInput(ctx, msg.Data[:msg.Len])
				if t.active {
					t.renderNow(ctx.NowTick())
				}
			}

		case tick := <-tickCh:
			if !t.active {
				continue
			}
			t.nowTick = tick
			t.onTick(tick)
			if t.active && t.dirty != 0 && tick >= t.nextRenderTick {
				t.renderNow(tick)
			}
		}
	}
}

func (t *Task) setActive(ctx *kernel.Context, active bool) {
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
	if t.vfs == nil && t.vfsCap.Valid() {
		t.vfs = vfsclient.New(t.vfsCap)
	}
	t.invalidate(dirtyAll)
	t.ensureWaterfallAlloc()
	t.renderNow(ctx.NowTick())
}

func (t *Task) unload() {
	t.active = false
	t.inbuf = nil
	t.scanActive = false
	t.scanNextTick = 0
	t.showMenu = false
	t.showHelp = false
	t.showFilters = false
}

func (t *Task) requestExit(ctx *kernel.Context) {
	if t.fb != nil {
		t.fb.ClearRGB(0, 0, 0)
		_ = t.fb.Present()
	}
	t.active = false
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

func (t *Task) invalidate(flags dirtyFlags) {
	t.dirty |= flags
}

func (t *Task) renderNow(now uint64) {
	t.renderDirty()
	t.nextRenderTick = now + renderIntervalTicks
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
	if t.showHelp {
		switch k.kind {
		case keyEsc:
			t.showHelp = false
			t.invalidate(dirtyOverlay | dirtyStatus)
		case keyRune:
			if k.r == 'h' || k.r == 'H' {
				t.showHelp = false
				t.invalidate(dirtyOverlay | dirtyStatus)
			}
		}
		return
	}

	if t.showMenu {
		switch k.kind {
		case keyEsc:
			t.showMenu = false
			t.invalidate(dirtyOverlay | dirtyHeader)
		case keyRune:
			if k.r == 'm' || k.r == 'M' {
				t.showMenu = false
				t.invalidate(dirtyOverlay | dirtyHeader)
			}
		}
		return
	}

	switch k.kind {
	case keyEsc:
		t.requestExit(ctx)
	case keyLeft:
		t.adjustLeft()
	case keyRight:
		t.adjustRight()
	case keyUp:
		t.adjustUp()
	case keyDown:
		t.adjustDown()
	case keyRune:
		switch k.r {
		case 'q':
			t.requestExit(ctx)
		case 's':
			now := ctx.NowTick()
			if t.scanActive {
				t.stopScan()
			} else {
				t.startScan(now)
			}
		case 'w':
			t.waterfallFrozen = !t.waterfallFrozen
			t.invalidate(dirtyStatus | dirtyWaterfall)
		case 'p':
			t.capturePaused = !t.capturePaused
			t.invalidate(dirtyStatus | dirtySniffer)
		case 'r':
			t.resetView()
		case 'm':
			t.showMenu = true
			t.invalidate(dirtyOverlay | dirtyHeader)
		case 't':
			t.cycleFocus()
		case 'c':
			t.selectedChannel++
			if t.selectedChannel > maxChannel {
				t.selectedChannel = 0
			}
			t.invalidate(dirtySpectrum | dirtyWaterfall | dirtyStatus)
		case 'f':
			t.showFilters = !t.showFilters
			t.invalidate(dirtyOverlay | dirtySniffer)
		case 'h':
			t.showHelp = true
			t.invalidate(dirtyOverlay | dirtyStatus)
		}
	}
}

func (t *Task) cycleFocus() {
	t.focus++
	if t.focus > focusProtocol {
		t.focus = focusSpectrum
	}
	t.invalidate(dirtyHeader)
}

func (t *Task) resetView() {
	t.focus = focusSpectrum
	t.selectedChannel = 37
	t.selectedSetting = 0
	t.showMenu = false
	t.showHelp = false
	t.showFilters = false

	t.scanChan = t.channelRangeLo
	t.scanNextTick = 0
	t.sweepCount = 0
	t.lastSweepTick = 0

	for i := range t.energyCur {
		t.energyCur[i] = 0
		t.energyAvg[i] = 0
		t.energyPeak[i] = 0
	}
	for i := range t.wfBuf {
		t.wfBuf[i] = 0
	}
	t.wfHead = 0
	t.invalidate(dirtyAll)
}

func (t *Task) adjustLeft() {
	switch t.focus {
	case focusSpectrum, focusWaterfall:
		t.selectedChannel--
		if t.selectedChannel < 0 {
			t.selectedChannel = maxChannel
		}
		t.invalidate(dirtySpectrum | dirtyWaterfall | dirtyStatus)
	case focusRFControl:
		t.adjustSetting(-1)
		t.invalidate(dirtyRFControl | dirtyStatus)
	}
}

func (t *Task) adjustRight() {
	switch t.focus {
	case focusSpectrum, focusWaterfall:
		t.selectedChannel++
		if t.selectedChannel > maxChannel {
			t.selectedChannel = 0
		}
		t.invalidate(dirtySpectrum | dirtyWaterfall | dirtyStatus)
	case focusRFControl:
		t.adjustSetting(+1)
		t.invalidate(dirtyRFControl | dirtyStatus)
	}
}

func (t *Task) adjustUp() {
	switch t.focus {
	case focusRFControl:
		if t.selectedSetting > 0 {
			t.selectedSetting--
		}
		t.invalidate(dirtyRFControl)
	case focusSniffer:
		t.invalidate(dirtySniffer)
	case focusProtocol:
		t.invalidate(dirtyProtocol)
	}
}

func (t *Task) adjustDown() {
	switch t.focus {
	case focusRFControl:
		if t.selectedSetting < int(rfSettingMax)-1 {
			t.selectedSetting++
		}
		t.invalidate(dirtyRFControl)
	case focusSniffer:
		t.invalidate(dirtySniffer)
	case focusProtocol:
		t.invalidate(dirtyProtocol)
	}
}
