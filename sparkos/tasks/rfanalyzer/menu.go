package rfanalyzer

type menuCategory uint8

const (
	menuView menuCategory = iota
	menuRF
	menuCapture
	menuDecode
	menuDisplay
	menuAdvanced
	menuHelp
)

var menuCategoryLabels = []string{
	"View",
	"RF",
	"Capture",
	"Decode",
	"Display",
	"Advanced",
	"Help",
}

type menuItemID uint16

const (
	menuItemFocusSpectrum menuItemID = iota
	menuItemFocusWaterfall
	menuItemFocusRFControl
	menuItemFocusSniffer
	menuItemFocusProtocol

	menuItemToggleScan
	menuItemToggleWaterfall
	menuItemToggleCapture
	menuItemResetView

	menuItemSetChannel
	menuItemSetRangeLo
	menuItemSetRangeHi
	menuItemSetDwell
	menuItemSetScanStep

	menuItemCycleRate
	menuItemCycleCRC
	menuItemToggleAutoAck
	menuItemCyclePower

	menuItemCyclePalette

	menuItemSavePreset
	menuItemLoadPreset

	menuItemOpenHelp
)

type menuItem struct {
	id    menuItemID
	label string
}

func menuItems(cat menuCategory) []menuItem {
	switch cat {
	case menuView:
		return []menuItem{
			{id: menuItemFocusSpectrum, label: "Focus Spectrum"},
			{id: menuItemFocusWaterfall, label: "Focus Waterfall"},
			{id: menuItemFocusRFControl, label: "Focus RF Control"},
			{id: menuItemFocusSniffer, label: "Focus Sniffer"},
			{id: menuItemFocusProtocol, label: "Focus Protocol"},
			{id: menuItemResetView, label: "Reset View"},
		}
	case menuRF:
		return []menuItem{
			{id: menuItemSetChannel, label: "Set Selected Channel…"},
			{id: menuItemSetRangeLo, label: "Set Range LO…"},
			{id: menuItemSetRangeHi, label: "Set Range HI…"},
			{id: menuItemSetDwell, label: "Set Dwell (ms)…"},
			{id: menuItemSetScanStep, label: "Set Scan Step…"},
			{id: menuItemCycleRate, label: "Data Rate"},
			{id: menuItemCycleCRC, label: "CRC"},
			{id: menuItemToggleAutoAck, label: "Auto-Ack"},
			{id: menuItemCyclePower, label: "Power"},
		}
	case menuCapture:
		return []menuItem{
			{id: menuItemToggleScan, label: "Start/Stop Scan"},
			{id: menuItemToggleWaterfall, label: "Freeze/Resume Waterfall"},
			{id: menuItemToggleCapture, label: "Pause/Resume Capture"},
		}
	case menuDecode:
		return []menuItem{
			{id: menuItemOpenHelp, label: "Protocol help (placeholder)"},
		}
	case menuDisplay:
		return []menuItem{
			{id: menuItemCyclePalette, label: "Waterfall Palette"},
		}
	case menuAdvanced:
		return []menuItem{
			{id: menuItemSavePreset, label: "Save Preset…"},
			{id: menuItemLoadPreset, label: "Load Preset…"},
		}
	case menuHelp:
		return []menuItem{
			{id: menuItemOpenHelp, label: "Hotkeys / Help"},
		}
	default:
		return nil
	}
}
