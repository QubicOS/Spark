package basic

func clipRunes(s string, maxCols int) string {
	if maxCols <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= maxCols {
		return s
	}
	if maxCols <= 1 {
		return string(r[:maxCols])
	}
	return string(append(r[:maxCols-1], '…'))
}
