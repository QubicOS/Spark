package proto

import "encoding/binary"

// ErrorPayload encodes a generic error response payload.
//
// Layout (little-endian):
//   - u16: code
//   - u16: ref kind (the request kind that failed)
//   - bytes: optional detail (service-defined)
func ErrorPayload(code ErrCode, ref Kind, detail []byte) []byte {
	buf := make([]byte, 4+len(detail))
	binary.LittleEndian.PutUint16(buf[0:2], uint16(code))
	binary.LittleEndian.PutUint16(buf[2:4], uint16(ref))
	copy(buf[4:], detail)
	return buf
}

// ErrorDetailWithRequestID prefixes service-specific detail with a request ID.
func ErrorDetailWithRequestID(requestID uint32, detail []byte) []byte {
	buf := make([]byte, 4+len(detail))
	binary.LittleEndian.PutUint32(buf[0:4], requestID)
	copy(buf[4:], detail)
	return buf
}

// DecodeErrorPayload decodes an ErrorPayload.
func DecodeErrorPayload(payload []byte) (code ErrCode, ref Kind, detail []byte, ok bool) {
	if len(payload) < 4 {
		return 0, 0, nil, false
	}
	code = ErrCode(binary.LittleEndian.Uint16(payload[0:2]))
	ref = Kind(binary.LittleEndian.Uint16(payload[2:4]))
	return code, ref, payload[4:], true
}

// DecodeErrorDetailWithRequestID decodes a requestID-prefixed error detail payload.
func DecodeErrorDetailWithRequestID(detail []byte) (requestID uint32, rest []byte, ok bool) {
	if len(detail) < 4 {
		return 0, nil, false
	}
	return binary.LittleEndian.Uint32(detail[0:4]), detail[4:], true
}
