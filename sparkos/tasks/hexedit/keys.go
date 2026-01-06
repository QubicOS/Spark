package hexedit

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
	case '3':
		if len(b) < 4 {
			return 0, key{}, false
		}
		if b[3] == '~' {
			return 4, key{kind: keyDelete}, true
		}
		return 1, key{kind: keyEsc}, true
	case '5':
		if len(b) < 4 {
			return 0, key{}, false
		}
		if b[3] == '~' {
			return 4, key{kind: keyPageUp}, true
		}
		return 1, key{kind: keyEsc}, true
	case '6':
		if len(b) < 4 {
			return 0, key{}, false
		}
		if b[3] == '~' {
			return 4, key{kind: keyPageDown}, true
		}
		return 1, key{kind: keyEsc}, true
	case '1':
		if len(b) < 4 {
			return 0, key{}, false
		}
		if b[3] == '~' {
			return 4, key{kind: keyHome}, true
		}
		return 1, key{kind: keyEsc}, true
	case '4':
		if len(b) < 4 {
			return 0, key{}, false
		}
		if b[3] == '~' {
			return 4, key{kind: keyEnd}, true
		}
		return 1, key{kind: keyEsc}, true
	default:
		return 1, key{kind: keyEsc}, true
	}
}
