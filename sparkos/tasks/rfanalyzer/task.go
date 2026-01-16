package rfanalyzer

import (
	"fmt"

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
	focusAnalysis
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

	recording           bool
	recordName          string
	recordPath          string
	recordBuf           []byte
	recordNextFlushTick uint64
	recordSweeps        uint32
	recordPackets       uint32
	recordBytes         uint32
	recordErr           string

	replayActive       bool
	replayPlaying      bool
	replaySpeed        int
	replayHostLastTick uint64
	replayNowTick      uint64
	replaySweepIdx     int
	replayPktLimit     int
	replayCfgIdx       int
	replay             *session
	replayErr          string
	replayPktCache     packet
	replayPktCacheSeq  uint32
	replayPktCacheOK   bool
	compare            *session
	compareErr         string

	rng uint32

	nextRenderTick uint64

	protoMode protocolMode

	packets     [maxPackets]packet
	pktHead     int
	pktCount    int
	pktSeq      uint32
	pktDropped  uint32
	pktSecStart uint64
	pktSecCount int
	pktsPerSec  int

	snifferSel    int
	snifferTop    int
	snifferSelSeq uint32

	filterCRC         filterCRC
	filterChannel     filterChannel
	filterMinLen      int
	filterMaxLen      int
	filterAddr        [5]byte
	filterAddrMask    [5]byte
	filterAddrLen     int
	filterPayload     [payloadPrefixBytes]byte
	filterPayloadMask [payloadPrefixBytes]byte
	filterPayloadLen  int
	filterAgeMs       int
	filterBurstMaxMs  int
	filterSel         int

	showMenu bool
	menuCat  menuCategory
	menuSel  int

	showHelp bool

	showFilters bool

	showPrompt   bool
	promptKind   promptKind
	promptTitle  string
	promptBuf    []rune
	promptCursor int
	promptErr    string

	annotations    [64]annotation
	annotHead      int
	annotCount     int
	annotPending   annotation
	annotLastTag   string
	annotLastDurMs int

	analysisView analysisView
	analysisSel  int
	analysisTop  int

	anaSweepCount uint32
	anaOccCount   [numChannels]uint32
	anaEnergySum  [numChannels]uint32
	anaChanPkt    [numChannels]uint32
	anaChanBad    [numChannels]uint32
	anaChanRetry  [numChannels]uint32

	anaHigh      [numChannels]bool
	anaLastRise  [numChannels]uint64
	anaRiseCount [numChannels]uint8
	anaRiseAvgMs [numChannels]uint32
	anaRiseMinMs [numChannels]uint32
	anaRiseMaxMs [numChannels]uint32

	bestHist     [64]bestChanEntry
	bestHead     int
	bestCount    int
	bestNextTick uint64

	devices     [maxDevices]deviceStat
	deviceCount int

	occHist      [occHistLen][occBytes]byte
	occHistHead  int
	occHistCount int

	dirty dirtyFlags
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
		replaySpeed:     1,
		protoMode:       protoDecoded,
		analysisView:    analysisChannels,
		filterCRC:       filterCRCAny,
		filterChannel:   filterChannelAll,
		menuCat:         menuRF,
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
			t.onTick(ctx, tick)
			if t.replayActive {
				t.updateReplayPacketCache(ctx)
			}
			t.tickPacketsPerSecond(tick)
			t.flushRecording(ctx, tick, false)
			if t.active && t.dirty != 0 && tick >= t.nextRenderTick {
				t.renderNow(tick)
			}
		}
	}
}

func (t *Task) setActive(ctx *kernel.Context, active bool) {
	if active == t.active {
		if !active {
			if t.recording {
				_ = t.stopRecording(ctx)
			}
			t.unload()
		}
		return
	}
	t.active = active
	if !t.active {
		if t.recording {
			_ = t.stopRecording(ctx)
		}
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
	t.replayActive = false
	t.replayPlaying = false
	t.replayHostLastTick = 0
	t.replayNowTick = 0
	t.replaySweepIdx = -1
	t.replayPktLimit = 0
	t.replayCfgIdx = -1
	t.replay = nil
	t.replayErr = ""
	t.replayPktCacheOK = false
	t.replayPktCacheSeq = 0
	t.compare = nil
	t.compareErr = ""
	t.showMenu = false
	t.showHelp = false
	t.showFilters = false
	t.showPrompt = false
	t.promptTitle = ""
	t.promptErr = ""
	t.promptBuf = nil
	t.promptCursor = 0
}

func (t *Task) requestExit(ctx *kernel.Context) {
	if t.recording {
		_ = t.stopRecording(ctx)
	}
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
	if t.showPrompt {
		t.handlePromptKey(ctx, k)
		return
	}

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
		t.handleMenuKey(ctx, k)
		return
	}

	if t.showFilters {
		t.handleFiltersKey(ctx, k)
		return
	}

	switch k.kind {
	case keyEsc:
		t.requestExit(ctx)
	case keyEnter:
		t.handleEnter(ctx)
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
			if t.replayActive {
				t.replayPlaying = !t.replayPlaying
				t.invalidate(dirtyStatus)
				return
			}
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
			if t.replayActive {
				t.replayPlaying = !t.replayPlaying
				t.invalidate(dirtyStatus)
				return
			}
			t.capturePaused = !t.capturePaused
			t.invalidate(dirtyStatus | dirtySniffer)
		case 'r':
			if t.replayActive {
				t.resetReplayView(ctx)
				return
			}
			t.resetView()
		case 'm':
			t.openMenu()
		case 't':
			t.cycleFocus()
		case 'c':
			t.openPrompt(promptSetChannel, "Set selected channel", fmt.Sprintf("%d", t.selectedChannel))
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
	if t.focus > focusAnalysis {
		t.focus = focusSpectrum
	}
	t.invalidate(dirtyHeader)
}

func (t *Task) prevAnalysisView() {
	t.analysisView = analysisView(wrapEnum(int(t.analysisView)-1, 7))
	t.analysisSel = 0
	t.analysisTop = 0
	t.invalidate(dirtyAnalysis)
}

func (t *Task) nextAnalysisView() {
	t.analysisView = analysisView(wrapEnum(int(t.analysisView)+1, 7))
	t.analysisSel = 0
	t.analysisTop = 0
	t.invalidate(dirtyAnalysis)
}

func (t *Task) handleEnter(ctx *kernel.Context) {
	switch t.focus {
	case focusRFControl:
		switch rfSetting(t.selectedSetting) {
		case rfSettingChanLo:
			t.openPrompt(promptSetRangeLo, "Set range LO", fmt.Sprintf("%d", t.channelRangeLo))
		case rfSettingChanHi:
			t.openPrompt(promptSetRangeHi, "Set range HI", fmt.Sprintf("%d", t.channelRangeHi))
		case rfSettingDwell:
			t.openPrompt(promptSetDwell, "Set dwell time (ms)", fmt.Sprintf("%d", t.dwellTimeMs))
		case rfSettingSpeed:
			t.openPrompt(promptSetScanStep, "Set scan step (1..10)", fmt.Sprintf("%d", clampInt(t.scanSpeedScalar, 1, 10)))
		case rfSettingRate:
			t.dataRate = rfDataRate(wrapEnum(int(t.dataRate)+1, 3))
			t.presetDirty = true
			t.recordConfig(ctx.NowTick())
			t.invalidate(dirtyRFControl | dirtyStatus)
		case rfSettingCRC:
			t.crcMode = rfCRCMode(wrapEnum(int(t.crcMode)+1, 3))
			t.presetDirty = true
			t.recordConfig(ctx.NowTick())
			t.invalidate(dirtyRFControl | dirtyStatus)
		case rfSettingAutoAck:
			t.autoAck = !t.autoAck
			t.presetDirty = true
			t.recordConfig(ctx.NowTick())
			t.invalidate(dirtyRFControl | dirtyStatus)
		case rfSettingPower:
			t.powerLevel = rfPowerLevel(wrapEnum(int(t.powerLevel)+1, 4))
			t.presetDirty = true
			t.recordConfig(ctx.NowTick())
			t.invalidate(dirtyRFControl | dirtyStatus)
		}
		return
	case focusSpectrum, focusWaterfall:
		t.openPrompt(promptSetChannel, fmt.Sprintf("Set channel (0..%d)", maxChannel), fmt.Sprintf("%d", t.selectedChannel))
		return
	case focusAnalysis:
		if t.analysisView == analysisChannels {
			top := t.topChannels(8)
			if t.analysisSel >= 0 && t.analysisSel < len(top) {
				t.selectedChannel = clampInt(top[t.analysisSel].ch, 0, maxChannel)
				t.recordConfig(ctx.NowTick())
				t.invalidate(dirtySpectrum | dirtyWaterfall | dirtyStatus | dirtyAnalysis)
			}
			return
		}
		if t.analysisView == analysisAnnotations {
			if !t.replayActive || t.replay == nil {
				return
			}
			notes := t.visibleAnnotations(t.replayNowTick, 8)
			if t.analysisSel < 0 || t.analysisSel >= len(notes) {
				return
			}
			a := notes[t.analysisSel]
			if a.startTick == 0 {
				return
			}
			t.replayNowTick = a.startTick
			t.replayHostLastTick = 0
			t.replayPlaying = false
			t.updateReplayPosition(ctx, t.replayNowTick, true)
			t.invalidate(dirtyAll)
		}
		return
	default:
		_ = ctx
		return
	}
}

func (t *Task) resetView() {
	t.focus = focusSpectrum
	t.selectedChannel = 37
	t.selectedSetting = 0
	t.showMenu = false
	t.showHelp = false
	t.showFilters = false
	t.showPrompt = false
	t.promptTitle = ""
	t.promptErr = ""
	t.promptBuf = nil
	t.promptCursor = 0
	t.annotPending = annotation{}

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
	t.resetAnalytics()
	t.invalidate(dirtyAll)
}

func (t *Task) resetReplayView(ctx *kernel.Context) {
	t.resetView()
	if ctx == nil || t.replay == nil {
		return
	}
	t.replayCfgIdx = -1
	t.replaySweepIdx = -1
	t.replayPktLimit = 0
	t.snifferSel = 0
	t.snifferTop = 0
	t.snifferSelSeq = 0
	t.replayPktCacheOK = false
	t.replayPktCacheSeq = 0
	t.replayHostLastTick = 0
	t.applyReplayConfigAt(t.replayNowTick)
	t.updateReplayPosition(ctx, t.replayNowTick, true)
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
	case focusAnalysis:
		t.prevAnalysisView()
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
	case focusAnalysis:
		t.nextAnalysisView()
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
		t.moveSnifferSelection(-1)
	case focusProtocol:
		t.invalidate(dirtyProtocol)
	case focusAnalysis:
		if t.analysisSel > 0 {
			t.analysisSel--
			t.invalidate(dirtyAnalysis)
		}
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
		t.moveSnifferSelection(+1)
	case focusProtocol:
		t.invalidate(dirtyProtocol)
	case focusAnalysis:
		t.analysisSel++
		t.invalidate(dirtyAnalysis)
	}
}
