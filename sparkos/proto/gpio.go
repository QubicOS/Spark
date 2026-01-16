package proto

import "encoding/binary"

// GPIOPinCaps declares supported operations for a pin.
type GPIOPinCaps uint8

const (
	GPIOPinCapInput GPIOPinCaps = 1 << iota
	GPIOPinCapOutput
	GPIOPinCapPullUp
	GPIOPinCapPullDown
)

// GPIOMode configures a pin direction.
type GPIOMode uint8

const (
	GPIOModeInput GPIOMode = iota
	GPIOModeOutput
)

// GPIOPull configures pull resistors.
type GPIOPull uint8

const (
	GPIOPullNone GPIOPull = iota
	GPIOPullUp
	GPIOPullDown
)

// GPIOListPayload encodes a MsgGPIOList request.
//
// Layout (little-endian):
//   - u32: request id
func GPIOListPayload(requestID uint32) []byte {
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf[0:4], requestID)
	return buf
}

func DecodeGPIOListPayload(b []byte) (requestID uint32, ok bool) {
	if len(b) != 4 {
		return 0, false
	}
	return binary.LittleEndian.Uint32(b[0:4]), true
}

// GPIOListRespPayload encodes a MsgGPIOListResp response.
//
// Layout (little-endian):
//   - u32: request id
//   - u8: done flag (0/1)
//   - u8: pin id
//   - u8: caps (GPIOPinCaps)
//   - u8: mode (GPIOMode)
//   - u8: pull (GPIOPull)
//   - u8: level (0/1)
func GPIOListRespPayload(requestID uint32, done bool, pinID uint8, caps GPIOPinCaps, mode GPIOMode, pull GPIOPull, level bool) []byte {
	buf := make([]byte, 10)
	binary.LittleEndian.PutUint32(buf[0:4], requestID)
	if done {
		buf[4] = 1
	}
	buf[5] = pinID
	buf[6] = uint8(caps)
	buf[7] = uint8(mode)
	buf[8] = uint8(pull)
	if level {
		buf[9] = 1
	}
	return buf
}

func DecodeGPIOListRespPayload(
	b []byte,
) (requestID uint32, done bool, pinID uint8, caps GPIOPinCaps, mode GPIOMode, pull GPIOPull, level bool, ok bool) {
	if len(b) != 10 {
		return 0, false, 0, 0, 0, 0, false, false
	}
	requestID = binary.LittleEndian.Uint32(b[0:4])
	done = b[4] != 0
	pinID = b[5]
	caps = GPIOPinCaps(b[6])
	mode = GPIOMode(b[7])
	pull = GPIOPull(b[8])
	level = b[9] != 0
	return requestID, done, pinID, caps, mode, pull, level, true
}

// GPIOConfigPayload encodes a MsgGPIOConfig request.
//
// Layout (little-endian):
//   - u32: request id
//   - u8: pin id
//   - u8: mode
//   - u8: pull
func GPIOConfigPayload(requestID uint32, pinID uint8, mode GPIOMode, pull GPIOPull) []byte {
	buf := make([]byte, 7)
	binary.LittleEndian.PutUint32(buf[0:4], requestID)
	buf[4] = pinID
	buf[5] = uint8(mode)
	buf[6] = uint8(pull)
	return buf
}

func DecodeGPIOConfigPayload(b []byte) (requestID uint32, pinID uint8, mode GPIOMode, pull GPIOPull, ok bool) {
	if len(b) != 7 {
		return 0, 0, 0, 0, false
	}
	return binary.LittleEndian.Uint32(b[0:4]), b[4], GPIOMode(b[5]), GPIOPull(b[6]), true
}

// GPIOConfigRespPayload encodes a MsgGPIOConfigResp response.
//
// Layout (little-endian):
//   - u32: request id
//   - u8: pin id
//   - u8: mode
//   - u8: pull
//   - u8: level
func GPIOConfigRespPayload(requestID uint32, pinID uint8, mode GPIOMode, pull GPIOPull, level bool) []byte {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint32(buf[0:4], requestID)
	buf[4] = pinID
	buf[5] = uint8(mode)
	buf[6] = uint8(pull)
	if level {
		buf[7] = 1
	}
	return buf
}

func DecodeGPIOConfigRespPayload(b []byte) (requestID uint32, pinID uint8, mode GPIOMode, pull GPIOPull, level bool, ok bool) {
	if len(b) != 8 {
		return 0, 0, 0, 0, false, false
	}
	requestID = binary.LittleEndian.Uint32(b[0:4])
	pinID = b[4]
	mode = GPIOMode(b[5])
	pull = GPIOPull(b[6])
	level = b[7] != 0
	return requestID, pinID, mode, pull, level, true
}

// GPIOWritePayload encodes a MsgGPIOWrite request.
//
// Layout (little-endian):
//   - u32: request id
//   - u8: pin id
//   - u8: level (0/1)
func GPIOWritePayload(requestID uint32, pinID uint8, level bool) []byte {
	buf := make([]byte, 6)
	binary.LittleEndian.PutUint32(buf[0:4], requestID)
	buf[4] = pinID
	if level {
		buf[5] = 1
	}
	return buf
}

func DecodeGPIOWritePayload(b []byte) (requestID uint32, pinID uint8, level bool, ok bool) {
	if len(b) != 6 {
		return 0, 0, false, false
	}
	return binary.LittleEndian.Uint32(b[0:4]), b[4], b[5] != 0, true
}

// GPIOWriteRespPayload encodes a MsgGPIOWriteResp response.
//
// Layout (little-endian):
//   - u32: request id
//   - u8: pin id
//   - u8: level (0/1)
func GPIOWriteRespPayload(requestID uint32, pinID uint8, level bool) []byte {
	return GPIOWritePayload(requestID, pinID, level)
}

func DecodeGPIOWriteRespPayload(b []byte) (requestID uint32, pinID uint8, level bool, ok bool) {
	return DecodeGPIOWritePayload(b)
}

// GPIOReadPayload encodes a MsgGPIORead request.
//
// Layout (little-endian):
//   - u32: request id
//   - u32: pin mask (bit i -> pin i)
func GPIOReadPayload(requestID uint32, mask uint32) []byte {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint32(buf[0:4], requestID)
	binary.LittleEndian.PutUint32(buf[4:8], mask)
	return buf
}

func DecodeGPIOReadPayload(b []byte) (requestID uint32, mask uint32, ok bool) {
	if len(b) != 8 {
		return 0, 0, false
	}
	return binary.LittleEndian.Uint32(b[0:4]), binary.LittleEndian.Uint32(b[4:8]), true
}

// GPIOReadRespPayload encodes a MsgGPIOReadResp response.
//
// Layout (little-endian):
//   - u32: request id
//   - u32: mask (echo)
//   - u32: levels (bit i -> level for pin i)
func GPIOReadRespPayload(requestID uint32, mask, levels uint32) []byte {
	buf := make([]byte, 12)
	binary.LittleEndian.PutUint32(buf[0:4], requestID)
	binary.LittleEndian.PutUint32(buf[4:8], mask)
	binary.LittleEndian.PutUint32(buf[8:12], levels)
	return buf
}

func DecodeGPIOReadRespPayload(b []byte) (requestID uint32, mask, levels uint32, ok bool) {
	if len(b) != 12 {
		return 0, 0, 0, false
	}
	return binary.LittleEndian.Uint32(b[0:4]), binary.LittleEndian.Uint32(b[4:8]), binary.LittleEndian.Uint32(b[8:12]), true
}
