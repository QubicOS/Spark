package rfanalyzer

type protocolMode uint8

const (
	protoDecoded protocolMode = iota
	protoRaw
)

func (m protocolMode) String() string {
	switch m {
	case protoDecoded:
		return "DECODED"
	case protoRaw:
		return "RAW"
	default:
		return "?"
	}
}

type filterCRC uint8

const (
	filterCRCAny filterCRC = iota
	filterCRCOK
	filterCRCBad
)

func (f filterCRC) String() string {
	switch f {
	case filterCRCAny:
		return "ANY"
	case filterCRCOK:
		return "OK"
	case filterCRCBad:
		return "BAD"
	default:
		return "?"
	}
}

type filterChannel uint8

const (
	filterChannelAll filterChannel = iota
	filterChannelSelected
	filterChannelRange
)

func (f filterChannel) String() string {
	switch f {
	case filterChannelAll:
		return "ALL"
	case filterChannelSelected:
		return "SEL"
	case filterChannelRange:
		return "RNG"
	default:
		return "?"
	}
}
