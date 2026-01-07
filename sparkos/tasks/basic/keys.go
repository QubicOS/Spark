package basic

import "unicode/utf8"

type keyKind uint8

const (
	keyRune keyKind = iota
	keyEnter
	keyBackspace
	keyTab
	keyEsc
	keyUp
	keyDown
	keyLeft
	keyRight
	keyDelete
	keyHome
	keyEnd
	keyCtrl
	keyPageUp
	keyPageDown
	keyF1
	keyF2
	keyF3
)

type key struct {
	kind keyKind
	r    rune
	ctrl byte
}

func nextKey(b []byte) (consumed int, k key, ok bool) {
	if len(b) == 0 {
		return 0, key{}, false
	}

	if b[0] == 0x1b {
		return parseEscapeKey(b)
	}

	switch b[0] {
	case '\r', '\n':
		return 1, key{kind: keyEnter}, true
	case 0x7f, 0x08:
		return 1, key{kind: keyBackspace}, true
	case '\t':
		return 1, key{kind: keyTab}, true
	}

	if b[0] < 0x20 {
		return 1, key{kind: keyCtrl, ctrl: b[0]}, true
	}
	if !utf8.FullRune(b) {
		return 0, key{}, false
	}
	r, sz := utf8.DecodeRune(b)
	if r == utf8.RuneError && sz == 1 {
		return 1, key{}, true
	}
	return sz, key{kind: keyRune, r: r}, true
}

func parseEscapeKey(b []byte) (consumed int, k key, ok bool) {
	if len(b) < 2 {
		return 1, key{kind: keyEsc}, true
	}
	// SS3 sequences (ESC O P/Q/R...) are commonly used for function keys.
	if b[1] == 'O' {
		if len(b) < 3 {
			return 0, key{}, false
		}
		switch b[2] {
		case 'P':
			return 3, key{kind: keyF1}, true
		case 'Q':
			return 3, key{kind: keyF2}, true
		case 'R':
			return 3, key{kind: keyF3}, true
		default:
			return 1, key{kind: keyEsc}, true
		}
	}
	if b[1] != '[' {
		return 1, key{kind: keyEsc}, true
	}
	if len(b) < 3 {
		return 0, key{}, false
	}

	switch b[2] {
	case 'A':
		return 3, key{kind: keyUp}, true
	case 'B':
		return 3, key{kind: keyDown}, true
	case 'C':
		return 3, key{kind: keyRight}, true
	case 'D':
		return 3, key{kind: keyLeft}, true
	case 'H':
		return 3, key{kind: keyHome}, true
	case 'F':
		return 3, key{kind: keyEnd}, true
	default:
		if b[2] < '0' || b[2] > '9' {
			return 1, key{kind: keyEsc}, true
		}

		n := 0
		i := 2
		for i < len(b) && b[i] >= '0' && b[i] <= '9' {
			n = n*10 + int(b[i]-'0')
			i++
		}
		if i >= len(b) {
			return 0, key{}, false
		}
		if b[i] != '~' {
			return 1, key{kind: keyEsc}, true
		}
		consumed = i + 1
		switch n {
		case 3:
			return consumed, key{kind: keyDelete}, true
		case 5:
			return consumed, key{kind: keyPageUp}, true
		case 6:
			return consumed, key{kind: keyPageDown}, true
		case 11:
			return consumed, key{kind: keyF1}, true
		case 12:
			return consumed, key{kind: keyF2}, true
		case 13:
			return consumed, key{kind: keyF3}, true
		default:
			return 1, key{kind: keyEsc}, true
		}
	}
}
