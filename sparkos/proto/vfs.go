package proto

import "encoding/binary"

// VFSEntryType is a directory entry type.
type VFSEntryType uint8

const (
	VFSEntryUnknown VFSEntryType = iota
	VFSEntryFile
	VFSEntryDir
)

// VFSWriteMode selects how writes are applied.
type VFSWriteMode uint8

const (
	VFSWriteTruncate VFSWriteMode = iota
	VFSWriteAppend
)

// VFSListPayload encodes a MsgVFSList request.
//
// Layout (little-endian):
//   - u32: request id
//   - u16: path length
//   - bytes: path (UTF-8)
func VFSListPayload(requestID uint32, path string) []byte {
	p := []byte(path)
	buf := make([]byte, 6+len(p))
	binary.LittleEndian.PutUint32(buf[0:4], requestID)
	binary.LittleEndian.PutUint16(buf[4:6], uint16(len(p)))
	copy(buf[6:], p)
	return buf
}

func DecodeVFSListPayload(b []byte) (requestID uint32, path string, ok bool) {
	if len(b) < 6 {
		return 0, "", false
	}
	requestID = binary.LittleEndian.Uint32(b[0:4])
	pathLen := int(binary.LittleEndian.Uint16(b[4:6]))
	if 6+pathLen != len(b) {
		return 0, "", false
	}
	return requestID, string(b[6:]), true
}

// VFSListRespPayload encodes a MsgVFSListResp response.
//
// Layout (little-endian):
//   - u32: request id
//   - u8: done flag (0/1)
//   - u8: entry type (VFSEntryType)
//   - u32: entry size (bytes, 0 for directories)
//   - u16: name length
//   - bytes: name (UTF-8)
func VFSListRespPayload(requestID uint32, done bool, typ VFSEntryType, size uint32, name string) []byte {
	n := []byte(name)
	buf := make([]byte, 12+len(n))
	binary.LittleEndian.PutUint32(buf[0:4], requestID)
	if done {
		buf[4] = 1
	}
	buf[5] = uint8(typ)
	binary.LittleEndian.PutUint32(buf[6:10], size)
	binary.LittleEndian.PutUint16(buf[10:12], uint16(len(n)))
	copy(buf[12:], n)
	return buf
}

func DecodeVFSListRespPayload(
	b []byte,
) (requestID uint32, done bool, typ VFSEntryType, size uint32, name string, ok bool) {
	if len(b) < 12 {
		return 0, false, 0, 0, "", false
	}
	requestID = binary.LittleEndian.Uint32(b[0:4])
	done = b[4] != 0
	typ = VFSEntryType(b[5])
	size = binary.LittleEndian.Uint32(b[6:10])
	nameLen := int(binary.LittleEndian.Uint16(b[10:12]))
	if 12+nameLen != len(b) {
		return 0, false, 0, 0, "", false
	}
	return requestID, done, typ, size, string(b[12:]), true
}

// VFSMkdirPayload encodes a MsgVFSMkdir request.
//
// Layout (little-endian):
//   - u32: request id
//   - u16: path length
//   - bytes: path (UTF-8)
func VFSMkdirPayload(requestID uint32, path string) []byte {
	p := []byte(path)
	buf := make([]byte, 6+len(p))
	binary.LittleEndian.PutUint32(buf[0:4], requestID)
	binary.LittleEndian.PutUint16(buf[4:6], uint16(len(p)))
	copy(buf[6:], p)
	return buf
}

func DecodeVFSMkdirPayload(b []byte) (requestID uint32, path string, ok bool) {
	if len(b) < 6 {
		return 0, "", false
	}
	requestID = binary.LittleEndian.Uint32(b[0:4])
	pathLen := int(binary.LittleEndian.Uint16(b[4:6]))
	if 6+pathLen != len(b) {
		return 0, "", false
	}
	return requestID, string(b[6:]), true
}

// VFSMkdirRespPayload encodes a MsgVFSMkdirResp response.
//
// Layout (little-endian):
//   - u32: request id
func VFSMkdirRespPayload(requestID uint32) []byte {
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf[0:4], requestID)
	return buf
}

func DecodeVFSMkdirRespPayload(b []byte) (requestID uint32, ok bool) {
	if len(b) != 4 {
		return 0, false
	}
	return binary.LittleEndian.Uint32(b[0:4]), true
}

// VFSRemovePayload encodes a MsgVFSRemove request.
//
// Layout (little-endian):
//   - u32: request id
//   - u16: path length
//   - bytes: path (UTF-8)
func VFSRemovePayload(requestID uint32, path string) []byte {
	p := []byte(path)
	buf := make([]byte, 6+len(p))
	binary.LittleEndian.PutUint32(buf[0:4], requestID)
	binary.LittleEndian.PutUint16(buf[4:6], uint16(len(p)))
	copy(buf[6:], p)
	return buf
}

func DecodeVFSRemovePayload(b []byte) (requestID uint32, path string, ok bool) {
	if len(b) < 6 {
		return 0, "", false
	}
	requestID = binary.LittleEndian.Uint32(b[0:4])
	pathLen := int(binary.LittleEndian.Uint16(b[4:6]))
	if 6+pathLen != len(b) {
		return 0, "", false
	}
	return requestID, string(b[6:]), true
}

// VFSRemoveRespPayload encodes a MsgVFSRemoveResp response.
//
// Layout (little-endian):
//   - u32: request id
func VFSRemoveRespPayload(requestID uint32) []byte {
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf[0:4], requestID)
	return buf
}

func DecodeVFSRemoveRespPayload(b []byte) (requestID uint32, ok bool) {
	if len(b) != 4 {
		return 0, false
	}
	return binary.LittleEndian.Uint32(b[0:4]), true
}

// VFSRenamePayload encodes a MsgVFSRename request.
//
// Layout (little-endian):
//   - u32: request id
//   - u16: old path length
//   - bytes: old path (UTF-8)
//   - u16: new path length
//   - bytes: new path (UTF-8)
func VFSRenamePayload(requestID uint32, oldPath, newPath string) []byte {
	oldB := []byte(oldPath)
	newB := []byte(newPath)
	buf := make([]byte, 8+len(oldB)+len(newB))
	binary.LittleEndian.PutUint32(buf[0:4], requestID)
	binary.LittleEndian.PutUint16(buf[4:6], uint16(len(oldB)))
	copy(buf[6:], oldB)
	base := 6 + len(oldB)
	binary.LittleEndian.PutUint16(buf[base:base+2], uint16(len(newB)))
	copy(buf[base+2:], newB)
	return buf
}

func DecodeVFSRenamePayload(b []byte) (requestID uint32, oldPath, newPath string, ok bool) {
	if len(b) < 8 {
		return 0, "", "", false
	}
	requestID = binary.LittleEndian.Uint32(b[0:4])
	oldLen := int(binary.LittleEndian.Uint16(b[4:6]))
	base := 6 + oldLen
	if base+2 > len(b) {
		return 0, "", "", false
	}
	newLen := int(binary.LittleEndian.Uint16(b[base : base+2]))
	if base+2+newLen != len(b) {
		return 0, "", "", false
	}
	oldPath = string(b[6:base])
	newPath = string(b[base+2:])
	return requestID, oldPath, newPath, true
}

// VFSRenameRespPayload encodes a MsgVFSRenameResp response.
//
// Layout (little-endian):
//   - u32: request id
func VFSRenameRespPayload(requestID uint32) []byte {
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf[0:4], requestID)
	return buf
}

func DecodeVFSRenameRespPayload(b []byte) (requestID uint32, ok bool) {
	if len(b) != 4 {
		return 0, false
	}
	return binary.LittleEndian.Uint32(b[0:4]), true
}

// VFSCopyPayload encodes a MsgVFSCopy request.
//
// Layout (little-endian):
//   - u32: request id
//   - u16: src path length
//   - bytes: src path (UTF-8)
//   - u16: dst path length
//   - bytes: dst path (UTF-8)
func VFSCopyPayload(requestID uint32, srcPath, dstPath string) []byte {
	src := []byte(srcPath)
	dst := []byte(dstPath)
	buf := make([]byte, 8+len(src)+len(dst))
	binary.LittleEndian.PutUint32(buf[0:4], requestID)
	binary.LittleEndian.PutUint16(buf[4:6], uint16(len(src)))
	copy(buf[6:], src)
	off := 6 + len(src)
	binary.LittleEndian.PutUint16(buf[off:off+2], uint16(len(dst)))
	copy(buf[off+2:], dst)
	return buf
}

func DecodeVFSCopyPayload(b []byte) (requestID uint32, srcPath, dstPath string, ok bool) {
	if len(b) < 8 {
		return 0, "", "", false
	}
	requestID = binary.LittleEndian.Uint32(b[0:4])
	srcLen := int(binary.LittleEndian.Uint16(b[4:6]))
	if 6+srcLen+2 > len(b) {
		return 0, "", "", false
	}
	srcPath = string(b[6 : 6+srcLen])
	off := 6 + srcLen
	dstLen := int(binary.LittleEndian.Uint16(b[off : off+2]))
	off += 2
	if off+dstLen != len(b) {
		return 0, "", "", false
	}
	dstPath = string(b[off:])
	return requestID, srcPath, dstPath, true
}

// VFSCopyRespPayload encodes a MsgVFSCopyResp response.
//
// Layout (little-endian):
//   - u32: request id
//   - u8 : done flag (0/1)
//   - u32: bytes copied
//   - u32: total bytes
func VFSCopyRespPayload(requestID uint32, done bool, copied, total uint32) []byte {
	buf := make([]byte, 13)
	binary.LittleEndian.PutUint32(buf[0:4], requestID)
	if done {
		buf[4] = 1
	}
	binary.LittleEndian.PutUint32(buf[5:9], copied)
	binary.LittleEndian.PutUint32(buf[9:13], total)
	return buf
}

func DecodeVFSCopyRespPayload(b []byte) (requestID uint32, done bool, copied, total uint32, ok bool) {
	if len(b) != 13 {
		return 0, false, 0, 0, false
	}
	requestID = binary.LittleEndian.Uint32(b[0:4])
	done = b[4] != 0
	copied = binary.LittleEndian.Uint32(b[5:9])
	total = binary.LittleEndian.Uint32(b[9:13])
	return requestID, done, copied, total, true
}

// VFSStatPayload encodes a MsgVFSStat request.
func VFSStatPayload(requestID uint32, path string) []byte {
	p := []byte(path)
	buf := make([]byte, 6+len(p))
	binary.LittleEndian.PutUint32(buf[0:4], requestID)
	binary.LittleEndian.PutUint16(buf[4:6], uint16(len(p)))
	copy(buf[6:], p)
	return buf
}

func DecodeVFSStatPayload(b []byte) (requestID uint32, path string, ok bool) {
	if len(b) < 6 {
		return 0, "", false
	}
	requestID = binary.LittleEndian.Uint32(b[0:4])
	pathLen := int(binary.LittleEndian.Uint16(b[4:6]))
	if 6+pathLen != len(b) {
		return 0, "", false
	}
	return requestID, string(b[6:]), true
}

// VFSStatRespPayload encodes a MsgVFSStatResp response.
//
// Layout (little-endian):
//   - u32: request id
//   - u8: entry type (VFSEntryType)
//   - u32: size
func VFSStatRespPayload(requestID uint32, typ VFSEntryType, size uint32) []byte {
	buf := make([]byte, 9)
	binary.LittleEndian.PutUint32(buf[0:4], requestID)
	buf[4] = uint8(typ)
	binary.LittleEndian.PutUint32(buf[5:9], size)
	return buf
}

func DecodeVFSStatRespPayload(b []byte) (requestID uint32, typ VFSEntryType, size uint32, ok bool) {
	if len(b) != 9 {
		return 0, 0, 0, false
	}
	requestID = binary.LittleEndian.Uint32(b[0:4])
	typ = VFSEntryType(b[4])
	size = binary.LittleEndian.Uint32(b[5:9])
	return requestID, typ, size, true
}

// VFSReadPayload encodes a MsgVFSRead request.
//
// Layout (little-endian):
//   - u32: request id
//   - u16: path length
//   - bytes: path (UTF-8)
//   - u32: offset
//   - u16: max bytes
func VFSReadPayload(requestID uint32, path string, off uint32, maxBytes uint16) []byte {
	p := []byte(path)
	buf := make([]byte, 12+len(p))
	binary.LittleEndian.PutUint32(buf[0:4], requestID)
	binary.LittleEndian.PutUint16(buf[4:6], uint16(len(p)))
	copy(buf[6:], p)
	base := 6 + len(p)
	binary.LittleEndian.PutUint32(buf[base:base+4], off)
	binary.LittleEndian.PutUint16(buf[base+4:base+6], maxBytes)
	return buf
}

// VFSReadPayloadInto encodes a MsgVFSRead request into dst and returns the used slice.
//
// It performs no heap allocations and returns ok=false if dst is too small.
func VFSReadPayloadInto(dst []byte, requestID uint32, path string, off uint32, maxBytes uint16) (payload []byte, ok bool) {
	if len(dst) < 12+len(path) {
		return nil, false
	}

	binary.LittleEndian.PutUint32(dst[0:4], requestID)
	binary.LittleEndian.PutUint16(dst[4:6], uint16(len(path)))
	copy(dst[6:], path)
	base := 6 + len(path)
	binary.LittleEndian.PutUint32(dst[base:base+4], off)
	binary.LittleEndian.PutUint16(dst[base+4:base+6], maxBytes)
	return dst[:base+6], true
}

func DecodeVFSReadPayload(
	b []byte,
) (requestID uint32, path string, off uint32, maxBytes uint16, ok bool) {
	if len(b) < 12 {
		return 0, "", 0, 0, false
	}
	requestID = binary.LittleEndian.Uint32(b[0:4])
	pathLen := int(binary.LittleEndian.Uint16(b[4:6]))
	base := 6 + pathLen
	if base+6 != len(b) {
		return 0, "", 0, 0, false
	}
	path = string(b[6:base])
	off = binary.LittleEndian.Uint32(b[base : base+4])
	maxBytes = binary.LittleEndian.Uint16(b[base+4 : base+6])
	return requestID, path, off, maxBytes, true
}

// VFSReadRespPayload encodes a MsgVFSReadResp response.
//
// Layout (little-endian):
//   - u32: request id
//   - u32: offset
//   - u8: eof flag (0/1)
//   - u16: data length
//   - bytes: data
func VFSReadRespPayload(requestID uint32, off uint32, eof bool, data []byte) []byte {
	buf := make([]byte, 11+len(data))
	binary.LittleEndian.PutUint32(buf[0:4], requestID)
	binary.LittleEndian.PutUint32(buf[4:8], off)
	if eof {
		buf[8] = 1
	}
	binary.LittleEndian.PutUint16(buf[9:11], uint16(len(data)))
	copy(buf[11:], data)
	return buf
}

func DecodeVFSReadRespPayload(
	b []byte,
) (requestID uint32, off uint32, eof bool, data []byte, ok bool) {
	if len(b) < 11 {
		return 0, 0, false, nil, false
	}
	requestID = binary.LittleEndian.Uint32(b[0:4])
	off = binary.LittleEndian.Uint32(b[4:8])
	eof = b[8] != 0
	dataLen := int(binary.LittleEndian.Uint16(b[9:11]))
	if 11+dataLen != len(b) {
		return 0, 0, false, nil, false
	}
	return requestID, off, eof, b[11:], true
}

// VFSWriteOpenPayload encodes a MsgVFSWriteOpen request.
//
// Layout (little-endian):
//   - u32: request id
//   - u8: mode (VFSWriteMode)
//   - u16: path length
//   - bytes: path (UTF-8)
func VFSWriteOpenPayload(requestID uint32, mode VFSWriteMode, path string) []byte {
	p := []byte(path)
	buf := make([]byte, 7+len(p))
	binary.LittleEndian.PutUint32(buf[0:4], requestID)
	buf[4] = uint8(mode)
	binary.LittleEndian.PutUint16(buf[5:7], uint16(len(p)))
	copy(buf[7:], p)
	return buf
}

func DecodeVFSWriteOpenPayload(b []byte) (requestID uint32, mode VFSWriteMode, path string, ok bool) {
	if len(b) < 7 {
		return 0, 0, "", false
	}
	requestID = binary.LittleEndian.Uint32(b[0:4])
	mode = VFSWriteMode(b[4])
	pathLen := int(binary.LittleEndian.Uint16(b[5:7]))
	if 7+pathLen != len(b) {
		return 0, 0, "", false
	}
	return requestID, mode, string(b[7:]), true
}

// VFSWriteChunkPayload encodes a MsgVFSWriteChunk request.
//
// Layout (little-endian):
//   - u32: request id
//   - u16: data length
//   - bytes: data
func VFSWriteChunkPayload(requestID uint32, data []byte) []byte {
	buf := make([]byte, 6+len(data))
	binary.LittleEndian.PutUint32(buf[0:4], requestID)
	binary.LittleEndian.PutUint16(buf[4:6], uint16(len(data)))
	copy(buf[6:], data)
	return buf
}

func DecodeVFSWriteChunkPayload(b []byte) (requestID uint32, data []byte, ok bool) {
	if len(b) < 6 {
		return 0, nil, false
	}
	requestID = binary.LittleEndian.Uint32(b[0:4])
	dataLen := int(binary.LittleEndian.Uint16(b[4:6]))
	if 6+dataLen != len(b) {
		return 0, nil, false
	}
	return requestID, b[6:], true
}

// VFSWriteClosePayload encodes a MsgVFSWriteClose request.
//
// Layout (little-endian):
//   - u32: request id
func VFSWriteClosePayload(requestID uint32) []byte {
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf[0:4], requestID)
	return buf
}

func DecodeVFSWriteClosePayload(b []byte) (requestID uint32, ok bool) {
	if len(b) != 4 {
		return 0, false
	}
	return binary.LittleEndian.Uint32(b[0:4]), true
}

// VFSWriteRespPayload encodes a MsgVFSWriteResp response.
//
// Layout (little-endian):
//   - u32: request id
//   - u8: done flag (0/1)
//   - u32: total bytes written (only valid when done=1)
func VFSWriteRespPayload(requestID uint32, done bool, n uint32) []byte {
	buf := make([]byte, 9)
	binary.LittleEndian.PutUint32(buf[0:4], requestID)
	if done {
		buf[4] = 1
	}
	binary.LittleEndian.PutUint32(buf[5:9], n)
	return buf
}

func DecodeVFSWriteRespPayload(b []byte) (requestID uint32, done bool, n uint32, ok bool) {
	if len(b) != 9 {
		return 0, false, 0, false
	}
	requestID = binary.LittleEndian.Uint32(b[0:4])
	done = b[4] != 0
	n = binary.LittleEndian.Uint32(b[5:9])
	return requestID, done, n, true
}
