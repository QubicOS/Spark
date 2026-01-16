package rfanalyzer

import "tinygo.org/x/tinyfont"

type fonter = tinyfont.Fonter

const (
	headerRows = 2
	statusRows = 2
)

type dirtyFlags uint16

const (
	dirtyHeader dirtyFlags = 1 << iota
	dirtySpectrum
	dirtyWaterfall
	dirtyRFControl
	dirtySniffer
	dirtyProtocol
	dirtyStatus
	dirtyOverlay

	dirtyAll = dirtyHeader | dirtySpectrum | dirtyWaterfall | dirtyRFControl | dirtySniffer | dirtyProtocol | dirtyStatus | dirtyOverlay
)

type rfDataRate uint8

const (
	rfRate250K rfDataRate = iota
	rfRate1M
	rfRate2M
)

func (r rfDataRate) String() string {
	switch r {
	case rfRate250K:
		return "250K"
	case rfRate1M:
		return "1M"
	case rfRate2M:
		return "2M"
	default:
		return "?"
	}
}

type rfCRCMode uint8

const (
	rfCRCOff rfCRCMode = iota
	rfCRC1B
	rfCRC2B
)

func (c rfCRCMode) String() string {
	switch c {
	case rfCRCOff:
		return "OFF"
	case rfCRC1B:
		return "1B"
	case rfCRC2B:
		return "2B"
	default:
		return "?"
	}
}

type rfPowerLevel uint8

const (
	rfPwrMin rfPowerLevel = iota
	rfPwrLow
	rfPwrHigh
	rfPwrMax
)

func (p rfPowerLevel) String() string {
	switch p {
	case rfPwrMin:
		return "MIN"
	case rfPwrLow:
		return "LOW"
	case rfPwrHigh:
		return "HIGH"
	case rfPwrMax:
		return "MAX"
	default:
		return "?"
	}
}

type rfSetting uint8

const (
	rfSettingChanLo rfSetting = iota
	rfSettingChanHi
	rfSettingDwell
	rfSettingSpeed
	rfSettingRate
	rfSettingCRC
	rfSettingAutoAck
	rfSettingPower
	rfSettingMax
)

const (
	maxChannel  = 125
	numChannels = 126
)

type wfPalette uint8

const (
	wfPaletteCyan wfPalette = iota
	wfPaletteFire
	wfPaletteGray
)

func (p wfPalette) String() string {
	switch p {
	case wfPaletteCyan:
		return "CYAN"
	case wfPaletteFire:
		return "FIRE"
	case wfPaletteGray:
		return "GRAY"
	default:
		return "?"
	}
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func (t *Task) adjustSetting(delta int) {
	if delta == 0 {
		return
	}
	switch rfSetting(t.selectedSetting) {
	case rfSettingChanLo:
		t.channelRangeLo = clampInt(t.channelRangeLo+delta, 0, maxChannel)
		if t.channelRangeLo > t.channelRangeHi {
			t.channelRangeHi = t.channelRangeLo
		}
	case rfSettingChanHi:
		t.channelRangeHi = clampInt(t.channelRangeHi+delta, 0, maxChannel)
		if t.channelRangeHi < t.channelRangeLo {
			t.channelRangeLo = t.channelRangeHi
		}
	case rfSettingDwell:
		t.dwellTimeMs = clampInt(t.dwellTimeMs+delta, 1, 50)
	case rfSettingSpeed:
		t.scanSpeedScalar = clampInt(t.scanSpeedScalar+delta, 1, 10)
	case rfSettingRate:
		t.dataRate = rfDataRate(wrapEnum(int(t.dataRate)+delta, 3))
	case rfSettingCRC:
		t.crcMode = rfCRCMode(wrapEnum(int(t.crcMode)+delta, 3))
	case rfSettingAutoAck:
		t.autoAck = !t.autoAck
	case rfSettingPower:
		t.powerLevel = rfPowerLevel(wrapEnum(int(t.powerLevel)+delta, 4))
	}
	t.presetDirty = true
	t.recordConfig(t.nowTick)
}

func wrapEnum(v, n int) int {
	if n <= 0 {
		return 0
	}
	for v < 0 {
		v += n
	}
	for v >= n {
		v -= n
	}
	return v
}
