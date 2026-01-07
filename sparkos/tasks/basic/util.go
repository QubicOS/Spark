package basic

import "unicode/utf8"

func decodeUTF8Rune(b []byte) (rune, int) {
	if len(b) == 0 {
		return 0, 0
	}
	r, n := utf8.DecodeRune(b)
	if r == utf8.RuneError && n == 1 {
		return 0, 0
	}
	return r, n
}
