package gpioscope

import (
	"fmt"

	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

type menuCategory uint8

const (
	menuView menuCategory = iota
	menuGPIO
	menuCapture
	menuTrigger
	menuDecode
	menuHelp
)

type menuItem struct {
	Label  string
	Action func(ctx *kernel.Context, t *Task)
}

func (t *Task) menuCats() []menuCategory {
	return []menuCategory{menuView, menuGPIO, menuCapture, menuTrigger, menuDecode, menuHelp}
}

func (t *Task) menuCatName(c menuCategory) string {
	switch c {
	case menuView:
		return "View"
	case menuGPIO:
		return "GPIO"
	case menuCapture:
		return "Capture"
	case menuTrigger:
		return "Trigger"
	case menuDecode:
		return "Decode"
	case menuHelp:
		return "Help"
	default:
		return "?"
	}
}

func (t *Task) menuItems(c menuCategory) []menuItem {
	switch c {
	case menuView:
		return []menuItem{
			{Label: t.modeLabel(modeGPIO), Action: func(_ *kernel.Context, t *Task) { t.mode = modeGPIO }},
			{Label: t.modeLabel(modeSignal), Action: func(_ *kernel.Context, t *Task) { t.mode = modeSignal }},
			{Label: t.modeLabel(modeProtocol), Action: func(_ *kernel.Context, t *Task) { t.mode = modeProtocol }},
			{Label: t.toggleLabel("Grid", t.showGrid), Action: func(_ *kernel.Context, t *Task) { t.showGrid = !t.showGrid }},
		}

	case menuGPIO:
		return []menuItem{
			{Label: "Toggle watch (*)", Action: func(_ *kernel.Context, t *Task) { t.toggleSelected() }},
			{Label: "Mode: INPUT", Action: func(ctx *kernel.Context, t *Task) { t.setMode(ctx, proto.GPIOModeInput) }},
			{Label: "Mode: OUTPUT", Action: func(ctx *kernel.Context, t *Task) { t.setMode(ctx, proto.GPIOModeOutput) }},
			{Label: "Pull: NONE", Action: func(ctx *kernel.Context, t *Task) { t.setPull(ctx, proto.GPIOPullNone) }},
			{Label: "Pull: UP", Action: func(ctx *kernel.Context, t *Task) { t.setPull(ctx, proto.GPIOPullUp) }},
			{Label: "Pull: DOWN", Action: func(ctx *kernel.Context, t *Task) { t.setPull(ctx, proto.GPIOPullDown) }},
			{Label: "Write: HIGH", Action: func(ctx *kernel.Context, t *Task) { t.writeLevel(ctx, true) }},
			{Label: "Write: LOW", Action: func(ctx *kernel.Context, t *Task) { t.writeLevel(ctx, false) }},
			{Label: "Write: TOGGLE", Action: func(ctx *kernel.Context, t *Task) { t.toggleLevel(ctx) }},
			{Label: "Pulse", Action: func(ctx *kernel.Context, t *Task) { t.pulse(ctx) }},
		}

	case menuCapture:
		return []menuItem{
			{Label: t.toggleLabel("Run", t.running), Action: func(ctx *kernel.Context, t *Task) { t.setRunning(ctx, !t.running) }},
			{Label: t.toggleLabel("Freeze", t.frozenActive), Action: func(_ *kernel.Context, t *Task) { t.toggleFreeze() }},
			{Label: t.toggleLabel("Single-shot", t.singleShot), Action: func(_ *kernel.Context, t *Task) { t.singleShot = !t.singleShot }},
			{Label: fmt.Sprintf("Period: %s", fmtHz(t.periodTicks)), Action: func(_ *kernel.Context, t *Task) { t.stepPeriod(+1) }},
			{Label: fmt.Sprintf("Zoom: %dx", t.samplesPerPx), Action: func(_ *kernel.Context, t *Task) { t.stepZoom(+1) }},
			{Label: "Reset view", Action: func(_ *kernel.Context, t *Task) { t.cursor = -1; t.scroll = 0 }},
		}

	case menuTrigger:
		return []menuItem{
			{Label: fmt.Sprintf("Mode: %s", t.triggerName()), Action: func(_ *kernel.Context, t *Task) { t.stepTrigger(+1) }},
			{Label: fmt.Sprintf("Pin: %d", t.triggerPinID), Action: func(_ *kernel.Context, t *Task) { t.stepTriggerPin(+1) }},
			{Label: t.toggleLabel("Arm", t.triggerArmed), Action: func(_ *kernel.Context, t *Task) { t.triggerArmed = !t.triggerArmed; t.triggered = false }},
		}

	case menuDecode:
		return []menuItem{
			{Label: t.protoLabel(protoNone), Action: func(_ *kernel.Context, t *Task) { t.pk = protoNone; t.decoded = nil }},
			{Label: t.protoLabel(protoUART), Action: func(_ *kernel.Context, t *Task) { t.pk = protoUART; t.decode() }},
			{Label: t.protoLabel(protoSPI), Action: func(_ *kernel.Context, t *Task) { t.pk = protoSPI; t.decode() }},
			{Label: t.protoLabel(protoI2C), Action: func(_ *kernel.Context, t *Task) { t.pk = protoI2C; t.decode() }},
			{Label: "Decode now", Action: func(_ *kernel.Context, t *Task) { t.decode() }},
		}

	case menuHelp:
		return []menuItem{
			{Label: "Show help", Action: func(_ *kernel.Context, t *Task) { t.showHelp = true }},
		}

	default:
		return nil
	}
}

func (t *Task) toggleLabel(name string, on bool) string {
	if on {
		return name + ": ON"
	}
	return name + ": OFF"
}

func (t *Task) modeLabel(m mode) string {
	name := "GPIO"
	switch m {
	case modeSignal:
		name = "Signal"
	case modeProtocol:
		name = "Protocol"
	}
	if t.mode == m {
		return name + "  <"
	}
	return name
}

func (t *Task) protoLabel(p protoKind) string {
	name := "None"
	switch p {
	case protoUART:
		name = "UART"
	case protoSPI:
		name = "SPI"
	case protoI2C:
		name = "I2C"
	}
	if t.pk == p {
		return name + "  <"
	}
	return name
}

func (t *Task) stepPeriod(dir int) {
	steps := []uint32{1, 2, 5, 10, 20, 50, 100}
	cur := 0
	for i, v := range steps {
		if v == t.periodTicks {
			cur = i
			break
		}
	}
	cur += dir
	if cur < 0 {
		cur = len(steps) - 1
	}
	if cur >= len(steps) {
		cur = 0
	}
	t.periodTicks = steps[cur]
}

func (t *Task) stepZoom(dir int) {
	steps := []int{1, 2, 4, 8, 16}
	cur := 0
	for i, v := range steps {
		if v == t.samplesPerPx {
			cur = i
			break
		}
	}
	cur += dir
	if cur < 0 {
		cur = len(steps) - 1
	}
	if cur >= len(steps) {
		cur = 0
	}
	t.samplesPerPx = steps[cur]
	if t.samplesPerPx < 1 {
		t.samplesPerPx = 1
	}
}

func (t *Task) stepTrigger(dir int) {
	all := []triggerKind{trigNone, trigRise, trigFall, trigHigh, trigLow}
	cur := 0
	for i, v := range all {
		if v == t.trigger {
			cur = i
			break
		}
	}
	cur += dir
	if cur < 0 {
		cur = len(all) - 1
	}
	if cur >= len(all) {
		cur = 0
	}
	t.trigger = all[cur]
}

func (t *Task) stepTriggerPin(dir int) {
	if len(t.pins) == 0 {
		t.triggerPinID = 0
		return
	}
	cur := 0
	for i := range t.pins {
		if t.pins[i].id == t.triggerPinID {
			cur = i
			break
		}
	}
	cur += dir
	if cur < 0 {
		cur = len(t.pins) - 1
	}
	if cur >= len(t.pins) {
		cur = 0
	}
	t.triggerPinID = t.pins[cur].id
}

func (t *Task) toggleFreeze() {
	if t.frozenActive {
		t.frozenActive = false
		t.frozen = nil
		t.msg = "live"
		return
	}
	t.frozenActive = true
	t.frozen = t.samplesForDecode()
	t.running = false
	t.msg = "frozen"
}
