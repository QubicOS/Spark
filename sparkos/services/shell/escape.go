package shell

type escAction uint8

const (
	escNone escAction = iota
	escUp
	escDown
	escRight
	escLeft
	escDelete
	escHome
	escEnd
)

func parseEscape(b []byte) (consumed int, action escAction, ok bool) {
	if len(b) < 2 || b[0] != 0x1b {
		return 0, escNone, true
	}
	if b[1] != '[' {
		if len(b) < 2 {
			return 0, escNone, false
		}
		return 2, escNone, true
	}
	if len(b) < 3 {
		return 0, escNone, false
	}
	switch b[2] {
	case 'A':
		return 3, escUp, true
	case 'B':
		return 3, escDown, true
	case 'C':
		return 3, escRight, true
	case 'D':
		return 3, escLeft, true
	case 'H':
		return 3, escHome, true
	case 'F':
		return 3, escEnd, true
	case '3':
		// CSI 3 ~ : Delete.
		if len(b) < 4 {
			return 0, escNone, false
		}
		if b[3] == '~' {
			return 4, escDelete, true
		}
		return consumeEscape(b), escNone, true
	case '1':
		// CSI 1 ~ : Home.
		if len(b) < 4 {
			return 0, escNone, false
		}
		if b[3] == '~' {
			return 4, escHome, true
		}
		return consumeEscape(b), escNone, true
	case '4':
		// CSI 4 ~ : End.
		if len(b) < 4 {
			return 0, escNone, false
		}
		if b[3] == '~' {
			return 4, escEnd, true
		}
		return consumeEscape(b), escNone, true
	default:
		return consumeEscape(b), escNone, true
	}
}

func consumeEscape(b []byte) int {
	if len(b) < 2 || b[0] != 0x1b {
		return 0
	}
	if b[1] == '[' {
		for i := 2; i < len(b); i++ {
			if b[i] >= 0x40 && b[i] <= 0x7e {
				return i + 1
			}
		}
		return len(b)
	}
	return 2
}
