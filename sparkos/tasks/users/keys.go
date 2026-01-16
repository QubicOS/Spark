package users

import "unicode/utf8"

type keyKind uint8

const (
	keyNone keyKind = iota
	keyEsc
	keyEnter
	keyBackspace
	keyLeft
	keyRight
	keyUp
	keyDown
	keyRune
)

type key struct {
	kind keyKind
	r    rune
}

func nextKey(b []byte) (consumed int, k key, ok bool) {
	if len(b) == 0 {
		return 0, key{}, false
	}

	switch b[0] {
	case 0x1b:
		if len(b) == 1 {
			return 1, key{kind: keyEsc}, true
		}
		if len(b) < 3 {
			return 0, key{}, false
		}
		if b[1] != '[' {
			return 1, key{kind: keyEsc}, true
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
		default:
			return 1, key{kind: keyEsc}, true
		}

	case '\r', '\n':
		return 1, key{kind: keyEnter}, true
	case 0x7f, 0x08:
		return 1, key{kind: keyBackspace}, true
	}

	if !utf8.FullRune(b) {
		return 0, key{}, false
	}
	r, sz := utf8.DecodeRune(b)
	if r == utf8.RuneError && sz == 1 {
		return 1, key{}, true
	}
	if r < 0x20 {
		return sz, key{}, true
	}
	return sz, key{kind: keyRune, r: r}, true
}
