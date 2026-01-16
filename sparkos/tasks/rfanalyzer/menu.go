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
	menuItemFocusAnalysis

	menuItemToggleScan
	menuItemToggleWaterfall
	menuItemToggleCapture
	menuItemToggleRecording
	menuItemAddAnnotationNow
	menuItemAddAnnotationSelected
	menuItemResetView

	menuItemLoadSession
	menuItemExitReplay
	menuItemReplayPlayPause
	menuItemReplaySeek
	menuItemReplaySpeed
	menuItemExportCSV
	menuItemExportPCAP
	menuItemExportRFPKT
	menuItemLoadCompareSession
	menuItemClearCompare

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

	menuItemToggleProtoMode

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
			{id: menuItemFocusAnalysis, label: "Focus Analysis"},
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
			{id: menuItemToggleRecording, label: "Start/Stop Recording…"},
			{id: menuItemAddAnnotationNow, label: "Add Annotation @Now…"},
			{id: menuItemAddAnnotationSelected, label: "Add Annotation @Selected…"},
			{id: menuItemLoadSession, label: "Load Session (Replay)…"},
			{id: menuItemLoadCompareSession, label: "Load Compare Session…"},
			{id: menuItemClearCompare, label: "Clear Compare Session"},
			{id: menuItemReplayPlayPause, label: "Replay Play/Pause"},
			{id: menuItemReplaySeek, label: "Replay Seek…"},
			{id: menuItemReplaySpeed, label: "Replay Speed"},
			{id: menuItemExportCSV, label: "Export CSV…"},
			{id: menuItemExportPCAP, label: "Export PCAP (DLT_USER0)…"},
			{id: menuItemExportRFPKT, label: "Export Raw Packets…"},
			{id: menuItemExitReplay, label: "Exit Replay (Live)"},
		}
	case menuDecode:
		return []menuItem{
			{id: menuItemToggleProtoMode, label: "Protocol View Mode"},
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
