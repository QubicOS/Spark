package archive

import (
	"encoding/binary"
	"fmt"
)

const (
	zipLocalSig   = 0x04034b50
	zipCentralSig = 0x02014b50
	zipEndSig     = 0x06054b50
)

func parseZipIndex(
	size uint32,
	readAt func(off uint32, n uint16) ([]byte, bool, error),
) ([]entry, error) {
	// Find EOCD in last 64k+22.
	var tailMax uint32 = 22 + 0xFFFF
	if size < tailMax {
		tailMax = size
	}
	start := size - tailMax
	tail, _, err := readAt(start, uint16(tailMax))
	if err != nil {
		return nil, fmt.Errorf("zip: read tail: %w", err)
	}

	eocdOff := findZipEOCD(tail)
	if eocdOff < 0 {
		return nil, fmt.Errorf("zip: missing EOCD")
	}
	e := tail[eocdOff:]
	if len(e) < 22 {
		return nil, fmt.Errorf("zip: short EOCD")
	}
	cdSize := u32le(e[12:16])
	cdOff := u32le(e[16:20])

	if cdOff+cdSize > size {
		return nil, fmt.Errorf("zip: central dir out of range")
	}
	cd, _, err := readAt(cdOff, uint16(cdSize))
	if err != nil {
		return nil, fmt.Errorf("zip: read central dir: %w", err)
	}
	if uint32(len(cd)) < cdSize {
		return nil, fmt.Errorf("zip: short central dir")
	}

	var out []entry
	cur := 0
	for cur+46 <= len(cd) {
		if u32le(cd[cur:cur+4]) != zipCentralSig {
			break
		}
		compMethod := u16le(cd[cur+10 : cur+12])
		compSize := u32le(cd[cur+20 : cur+24])
		uncompSize := u32le(cd[cur+24 : cur+28])
		nameLen := int(u16le(cd[cur+28 : cur+30]))
		extraLen := int(u16le(cd[cur+30 : cur+32]))
		cmtLen := int(u16le(cd[cur+32 : cur+34]))
		localOff := u32le(cd[cur+42 : cur+46])
		cur += 46
		if cur+nameLen+extraLen+cmtLen > len(cd) {
			return nil, fmt.Errorf("zip: central dir truncated")
		}
		name := sanitizeRelPath(string(cd[cur : cur+nameLen]))
		cur += nameLen + extraLen + cmtLen
		if name == "" {
			continue
		}

		entType := entryFile
		if stringsHasSuffix(name, "/") {
			entType = entryDir
		}

		dataOff, _, err := zipLocalDataOffset(size, readAt, localOff)
		if err != nil {
			return nil, fmt.Errorf("zip: %q: %w", name, err)
		}
		if dataOff+compSize > size {
			return nil, fmt.Errorf("zip: %q: data out of range", name)
		}

		out = append(out, entry{
			name:       name,
			typ:        entType,
			size:       uncompSize,
			dataOff:    dataOff,
			compSize:   compSize,
			compMethod: compMethod,
		})
	}

	return out, nil
}

func findZipEOCD(b []byte) int {
	// Search backwards for signature.
	for i := len(b) - 22; i >= 0; i-- {
		if u32le(b[i:i+4]) == zipEndSig {
			return i
		}
	}
	return -1
}

func zipLocalDataOffset(
	size uint32,
	readAt func(off uint32, n uint16) ([]byte, bool, error),
	localOff uint32,
) (dataOff uint32, extraLen uint16, err error) {
	hdr, _, err := readAt(localOff, 30)
	if err != nil {
		return 0, 0, err
	}
	if len(hdr) < 30 {
		return 0, 0, fmt.Errorf("short local header")
	}
	if u32le(hdr[0:4]) != zipLocalSig {
		return 0, 0, fmt.Errorf("bad local signature")
	}
	nameLen := u16le(hdr[26:28])
	extraLen = u16le(hdr[28:30])
	off := localOff + 30 + uint32(nameLen) + uint32(extraLen)
	if off > size {
		return 0, 0, fmt.Errorf("local header out of range")
	}
	return off, extraLen, nil
}

func zipEntryIsSupported(e entry) error {
	if e.typ == entryDir {
		return nil
	}
	if e.compMethod == 0 {
		return nil
	}
	return errUnsupportedZipMethod
}

// zipStore builds a minimal zip with "store" method.
// It returns (localHeadersAndData, centralDir, eocd) as a single byte slice.
func zipStore(build func(add func(name string, data []byte) error) error) ([]byte, error) {
	type central struct {
		name  string
		crc32 uint32
		size  uint32
		off   uint32
		isDir bool
	}

	var (
		out     []byte
		entries []central
	)

	add := func(name string, data []byte) error {
		name = sanitizeRelPath(name)
		if name == "" {
			return fmt.Errorf("zip: empty name")
		}
		isDir := stringsHasSuffix(name, "/")
		if isDir && len(data) != 0 {
			data = nil
		}
		off := uint32(len(out))

		// Local file header.
		// signature + version + flags + method + time/date + crc + sizes + name len + extra len.
		var hdr [30]byte
		binary.LittleEndian.PutUint32(hdr[0:4], zipLocalSig)
		binary.LittleEndian.PutUint16(hdr[4:6], 20)
		binary.LittleEndian.PutUint16(hdr[6:8], 0)
		binary.LittleEndian.PutUint16(hdr[8:10], 0) // store
		// time/date left as 0.
		binary.LittleEndian.PutUint32(hdr[14:18], crc32IEEE(data))
		binary.LittleEndian.PutUint32(hdr[18:22], uint32(len(data)))
		binary.LittleEndian.PutUint32(hdr[22:26], uint32(len(data)))
		binary.LittleEndian.PutUint16(hdr[26:28], uint16(len(name)))
		binary.LittleEndian.PutUint16(hdr[28:30], 0)

		out = append(out, hdr[:]...)
		out = append(out, []byte(name)...)
		out = append(out, data...)

		entries = append(entries, central{
			name:  name,
			crc32: crc32IEEE(data),
			size:  uint32(len(data)),
			off:   off,
			isDir: isDir,
		})
		return nil
	}

	if err := build(add); err != nil {
		return nil, err
	}

	centralOff := uint32(len(out))
	var cd []byte
	for _, e := range entries {
		// Central directory header (46 bytes).
		var h [46]byte
		binary.LittleEndian.PutUint32(h[0:4], zipCentralSig)
		binary.LittleEndian.PutUint16(h[4:6], 20)  // made by
		binary.LittleEndian.PutUint16(h[6:8], 20)  // needed
		binary.LittleEndian.PutUint16(h[8:10], 0)  // flags
		binary.LittleEndian.PutUint16(h[10:12], 0) // method store
		binary.LittleEndian.PutUint32(h[16:20], e.crc32)
		binary.LittleEndian.PutUint32(h[20:24], e.size)
		binary.LittleEndian.PutUint32(h[24:28], e.size)
		binary.LittleEndian.PutUint16(h[28:30], uint16(len(e.name)))
		binary.LittleEndian.PutUint16(h[30:32], 0) // extra
		binary.LittleEndian.PutUint16(h[32:34], 0) // comment
		binary.LittleEndian.PutUint16(h[34:36], 0) // disk
		binary.LittleEndian.PutUint16(h[36:38], 0) // int attr
		extAttr := uint32(0)
		if e.isDir {
			extAttr = 0x10
		}
		binary.LittleEndian.PutUint32(h[38:42], extAttr)
		binary.LittleEndian.PutUint32(h[42:46], e.off)
		cd = append(cd, h[:]...)
		cd = append(cd, []byte(e.name)...)
	}
	cdSize := uint32(len(cd))
	out = append(out, cd...)

	// EOCD.
	var eocd [22]byte
	binary.LittleEndian.PutUint32(eocd[0:4], zipEndSig)
	binary.LittleEndian.PutUint16(eocd[8:10], uint16(len(entries)))
	binary.LittleEndian.PutUint16(eocd[10:12], uint16(len(entries)))
	binary.LittleEndian.PutUint32(eocd[12:16], cdSize)
	binary.LittleEndian.PutUint32(eocd[16:20], centralOff)
	binary.LittleEndian.PutUint16(eocd[20:22], 0)
	out = append(out, eocd[:]...)
	return out, nil
}

func crc32IEEE(b []byte) uint32 {
	// Very small CRC32 implementation (IEEE).
	const poly = 0xedb88320
	var crc uint32 = 0xffffffff
	for i := 0; i < len(b); i++ {
		crc ^= uint32(b[i])
		for j := 0; j < 8; j++ {
			if crc&1 != 0 {
				crc = (crc >> 1) ^ poly
			} else {
				crc >>= 1
			}
		}
	}
	return ^crc
}
