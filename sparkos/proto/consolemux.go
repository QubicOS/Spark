package proto

import "encoding/binary"

// MuxStatusPayload encodes a consolemux status request.
//
// Payload format (little-endian):
//
//	u32 requestID
//
// The reply capability must be transferred in Message.Cap.
func MuxStatusPayload(requestID uint32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, requestID)
	return b
}

func DecodeMuxStatusPayload(b []byte) (requestID uint32, ok bool) {
	if len(b) != 4 {
		return 0, false
	}
	return binary.LittleEndian.Uint32(b), true
}

// MuxStatusRespPayload encodes a consolemux status response.
//
// Payload format (little-endian):
//
//	u32 requestID
//	u8  activeAppID
//	u8  focusApp   (0/1)
//	u8  hasApp     (0/1)  // selected app capability exists in this build
func MuxStatusRespPayload(requestID uint32, activeAppID AppID, focusApp bool, hasApp bool) []byte {
	b := make([]byte, 7)
	binary.LittleEndian.PutUint32(b[0:4], requestID)
	b[4] = byte(activeAppID)
	if focusApp {
		b[5] = 1
	}
	if hasApp {
		b[6] = 1
	}
	return b
}

func DecodeMuxStatusRespPayload(b []byte) (requestID uint32, activeAppID AppID, focusApp bool, hasApp bool, ok bool) {
	if len(b) != 7 {
		return 0, 0, false, false, false
	}
	return binary.LittleEndian.Uint32(b[0:4]), AppID(b[4]), b[5] != 0, b[6] != 0, true
}
