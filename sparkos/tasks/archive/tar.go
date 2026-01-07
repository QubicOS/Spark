package archive

import (
	"bytes"
	"errors"
	"fmt"
)

const tarBlockSize = 512

func parseTarIndex(
	size uint32,
	readAt func(off uint32, n uint16) ([]byte, bool, error),
) ([]entry, error) {
	var out []entry
	var off uint32

	var block [tarBlockSize]byte
	for off+tarBlockSize <= size {
		b, _, err := readAt(off, tarBlockSize)
		if err != nil {
			return nil, fmt.Errorf("tar: read header at %d: %w", off, err)
		}
		if len(b) < tarBlockSize {
			return nil, fmt.Errorf("tar: short read at %d", off)
		}
		copy(block[:], b[:tarBlockSize])

		if isAllZero(block[:]) {
			break
		}

		name := stringsTrimNul(string(block[0:100]))
		prefix := stringsTrimNul(string(block[345:500]))
		if prefix != "" {
			name = prefix + "/" + name
		}
		name = sanitizeRelPath(name)
		if name == "" {
			return nil, errors.New("tar: empty name")
		}

		typ := block[156]
		sz, err := parseOctalUint32(block[124:136])
		if err != nil {
			return nil, fmt.Errorf("tar: parse size for %q: %w", name, err)
		}

		entType := entryFile
		if typ == '5' || (typ == 0 && stringsHasSuffix(name, "/")) {
			entType = entryDir
			if !stringsHasSuffix(name, "/") {
				name += "/"
			}
			sz = 0
		}

		dataOff := off + tarBlockSize
		out = append(out, entry{
			name:    name,
			typ:     entType,
			size:    sz,
			dataOff: dataOff,
		})

		off = dataOff + roundUp512(sz)
		if off%tarBlockSize != 0 {
			off += tarBlockSize - (off % tarBlockSize)
		}
	}

	return out, nil
}

func isAllZero(b []byte) bool {
	for i := range b {
		if b[i] != 0 {
			return false
		}
	}
	return true
}

func stringsTrimNul(s string) string {
	i := bytes.IndexByte([]byte(s), 0)
	if i >= 0 {
		s = s[:i]
	}
	return s
}

func stringsHasSuffix(s, suf string) bool {
	if len(suf) == 0 {
		return true
	}
	if len(s) < len(suf) {
		return false
	}
	return s[len(s)-len(suf):] == suf
}

func parseOctalUint32(b []byte) (uint32, error) {
	var n uint32
	seen := false
	for i := 0; i < len(b); i++ {
		ch := b[i]
		if ch == 0 || ch == ' ' {
			continue
		}
		if ch < '0' || ch > '7' {
			return 0, fmt.Errorf("invalid octal digit %q", ch)
		}
		seen = true
		n = n*8 + uint32(ch-'0')
	}
	if !seen {
		return 0, nil
	}
	return n, nil
}

func roundUp512(n uint32) uint32 {
	if n%tarBlockSize == 0 {
		return n
	}
	return n + (tarBlockSize - (n % tarBlockSize))
}
