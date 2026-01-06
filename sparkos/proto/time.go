package proto

import "encoding/binary"

// SleepPayload encodes a MsgSleep request payload.
//
// Layout (little-endian):
//   - u32: requestID
//   - u32: dt ticks
func SleepPayload(requestID uint32, dt uint32) []byte {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint32(buf[0:4], requestID)
	binary.LittleEndian.PutUint32(buf[4:8], dt)
	return buf
}

// DecodeSleepPayload decodes a SleepPayload.
func DecodeSleepPayload(payload []byte) (requestID uint32, dt uint32, ok bool) {
	if len(payload) < 8 {
		return 0, 0, false
	}
	requestID = binary.LittleEndian.Uint32(payload[0:4])
	dt = binary.LittleEndian.Uint32(payload[4:8])
	return requestID, dt, true
}

// WakePayload encodes a MsgWake response payload.
//
// Layout (little-endian):
//   - u32: requestID
func WakePayload(requestID uint32) []byte {
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf[0:4], requestID)
	return buf
}

// DecodeWakePayload decodes a WakePayload.
func DecodeWakePayload(payload []byte) (requestID uint32, ok bool) {
	if len(payload) < 4 {
		return 0, false
	}
	return binary.LittleEndian.Uint32(payload[0:4]), true
}
