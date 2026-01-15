package rfanalyzer

import (
	"fmt"

	"spark/sparkos/kernel"
)

func (t *Task) openMenu() {
	t.showMenu = true
	t.menuSel = 0
	t.invalidate(dirtyOverlay | dirtyHeader | dirtyStatus)
}

func (t *Task) closeMenu() {
	if !t.showMenu {
		return
	}
	t.showMenu = false
	t.invalidate(dirtyOverlay | dirtyHeader | dirtyStatus)
}

func (t *Task) handleMenuKey(ctx *kernel.Context, k key) {
	switch k.kind {
	case keyEsc:
		t.closeMenu()
		return
	case keyRune:
		if k.r == 'm' || k.r == 'M' {
			t.closeMenu()
			return
		}
	case keyLeft:
		if t.menuCat == 0 {
			t.menuCat = menuHelp
		} else {
			t.menuCat--
		}
		t.menuSel = 0
		t.invalidate(dirtyOverlay)
		return
	case keyRight:
		if t.menuCat >= menuHelp {
			t.menuCat = 0
		} else {
			t.menuCat++
		}
		t.menuSel = 0
		t.invalidate(dirtyOverlay)
		return
	case keyUp, keyDown:
		items := menuItems(t.menuCat)
		if len(items) == 0 {
			return
		}
		if k.kind == keyUp {
			if t.menuSel <= 0 {
				t.menuSel = len(items) - 1
			} else {
				t.menuSel--
			}
		} else {
			if t.menuSel >= len(items)-1 {
				t.menuSel = 0
			} else {
				t.menuSel++
			}
		}
		t.invalidate(dirtyOverlay)
		return
	case keyEnter:
		items := menuItems(t.menuCat)
		if len(items) == 0 || t.menuSel < 0 || t.menuSel >= len(items) {
			return
		}
		t.activateMenuItem(ctx, items[t.menuSel].id)
		return
	}
}

func (t *Task) activateMenuItem(ctx *kernel.Context, id menuItemID) {
	switch id {
	case menuItemFocusSpectrum:
		t.focus = focusSpectrum
		t.closeMenu()
		t.invalidate(dirtyHeader)
	case menuItemFocusWaterfall:
		t.focus = focusWaterfall
		t.closeMenu()
		t.invalidate(dirtyHeader)
	case menuItemFocusRFControl:
		t.focus = focusRFControl
		t.closeMenu()
		t.invalidate(dirtyHeader)
	case menuItemFocusSniffer:
		t.focus = focusSniffer
		t.closeMenu()
		t.invalidate(dirtyHeader)
	case menuItemFocusProtocol:
		t.focus = focusProtocol
		t.closeMenu()
		t.invalidate(dirtyHeader)

	case menuItemToggleScan:
		now := ctx.NowTick()
		if t.scanActive {
			t.stopScan()
		} else {
			t.startScan(now)
		}
		t.invalidate(dirtyOverlay)

	case menuItemToggleWaterfall:
		t.waterfallFrozen = !t.waterfallFrozen
		t.invalidate(dirtyStatus | dirtyWaterfall | dirtyOverlay)

	case menuItemToggleCapture:
		t.capturePaused = !t.capturePaused
		t.invalidate(dirtyStatus | dirtySniffer | dirtyOverlay)

	case menuItemResetView:
		t.resetView()
		t.invalidate(dirtyOverlay)

	case menuItemSetChannel:
		t.openPrompt(promptSetChannel, fmt.Sprintf("Set channel (0..%d)", maxChannel), fmt.Sprintf("%d", t.selectedChannel))
		t.closeMenu()
	case menuItemSetRangeLo:
		t.openPrompt(promptSetRangeLo, "Set range LO", fmt.Sprintf("%d", t.channelRangeLo))
		t.closeMenu()
	case menuItemSetRangeHi:
		t.openPrompt(promptSetRangeHi, "Set range HI", fmt.Sprintf("%d", t.channelRangeHi))
		t.closeMenu()
	case menuItemSetDwell:
		t.openPrompt(promptSetDwell, "Set dwell time (ms)", fmt.Sprintf("%d", t.dwellTimeMs))
		t.closeMenu()
	case menuItemSetScanStep:
		t.openPrompt(promptSetScanStep, "Set scan step (1..10)", fmt.Sprintf("%d", clampInt(t.scanSpeedScalar, 1, 10)))
		t.closeMenu()

	case menuItemCycleRate:
		t.dataRate = rfDataRate(wrapEnum(int(t.dataRate)+1, 3))
		t.presetDirty = true
		t.scanNextTick = 0
		t.invalidate(dirtyRFControl | dirtySpectrum | dirtyStatus | dirtyOverlay)

	case menuItemCycleCRC:
		t.crcMode = rfCRCMode(wrapEnum(int(t.crcMode)+1, 3))
		t.presetDirty = true
		t.invalidate(dirtyRFControl | dirtyStatus | dirtyOverlay)

	case menuItemToggleAutoAck:
		t.autoAck = !t.autoAck
		t.presetDirty = true
		t.invalidate(dirtyRFControl | dirtyStatus | dirtyOverlay)

	case menuItemCyclePower:
		t.powerLevel = rfPowerLevel(wrapEnum(int(t.powerLevel)+1, 4))
		t.presetDirty = true
		t.invalidate(dirtyRFControl | dirtyStatus | dirtyOverlay)

	case menuItemCyclePalette:
		t.wfPalette = wfPalette(wrapEnum(int(t.wfPalette)+1, 3))
		t.rebuildWaterfallPalette()
		t.presetDirty = true
		t.invalidate(dirtyWaterfall | dirtyStatus | dirtyOverlay)

	case menuItemSavePreset:
		initial := t.activePreset
		if initial == "" {
			initial = "scan"
		}
		t.openPrompt(promptSavePreset, "Save preset name", initial)
		t.closeMenu()

	case menuItemLoadPreset:
		initial := t.activePreset
		if initial == "" {
			initial = "scan"
		}
		t.openPrompt(promptLoadPreset, "Load preset name", initial)
		t.closeMenu()

	case menuItemOpenHelp:
		t.showHelp = true
		t.closeMenu()
		t.invalidate(dirtyOverlay | dirtyStatus)
	}
}
