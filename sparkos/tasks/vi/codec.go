//go:build spark_vi

package vi

import (
	"strings"
)

func decodeLines(data []byte) [][]rune {
	s := string(data)
	parts := strings.Split(s, "\n")
	if len(parts) > 0 && parts[len(parts)-1] == "" && strings.HasSuffix(s, "\n") {
		parts = parts[:len(parts)-1]
	}
	if len(parts) == 0 {
		return [][]rune{{}}
	}

	lines := make([][]rune, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSuffix(p, "\r")
		lines = append(lines, []rune(p))
	}
	if len(lines) == 0 {
		lines = [][]rune{{}}
	}
	return lines
}

func encodeLines(lines [][]rune) []byte {
	if len(lines) == 0 {
		return []byte("\n")
	}

	var b strings.Builder
	for i, ln := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(string(ln))
	}
	b.WriteByte('\n')
	return []byte(b.String())
}
