package archive

import (
	"fmt"
)

func tarHeader(rel string, size uint32, isDir bool) [tarBlockSize]byte {
	rel = sanitizeRelPath(rel)
	if isDir && rel != "" && !stringsHasSuffix(rel, "/") {
		rel += "/"
	}

	var h [tarBlockSize]byte
	name := rel
	prefix := ""
	if len(name) > 100 {
		// Try to split into prefix/name at the last '/'.
		if i := lastIndexByte(name, '/'); i >= 0 {
			prefix = name[:i]
			name = name[i+1:]
		}
	}
	copy(h[0:100], []byte(name))
	copy(h[345:500], []byte(prefix))

	mode := uint32(0644)
	typeflag := byte('0')
	if isDir {
		mode = 0755
		typeflag = '5'
		size = 0
	}

	writeOctal(h[100:108], mode)
	writeOctal(h[108:116], 0) // uid
	writeOctal(h[116:124], 0) // gid
	writeOctal(h[124:136], size)
	writeOctal(h[136:148], 0) // mtime

	for i := 148; i < 156; i++ {
		h[i] = ' '
	}
	h[156] = typeflag

	copy(h[257:263], []byte("ustar\x00"))
	copy(h[263:265], []byte("00"))

	sum := tarChecksum(h[:])
	writeOctal(h[148:156], sum)
	h[155] = ' '
	return h
}

func tarChecksum(b []byte) uint32 {
	var sum uint32
	for i := 0; i < len(b); i++ {
		sum += uint32(b[i])
	}
	return sum
}

func writeOctal(dst []byte, v uint32) {
	// Write as octal with trailing NUL, padded with leading zeros/spaces.
	// Most tar readers accept either NUL or space terminator.
	if len(dst) == 0 {
		return
	}
	n := len(dst)
	for i := range dst {
		dst[i] = 0
	}
	s := fmt.Sprintf("%0*o", n-1, v)
	copy(dst, []byte(s))
	dst[n-1] = 0
}

func lastIndexByte(s string, b byte) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == b {
			return i
		}
	}
	return -1
}
