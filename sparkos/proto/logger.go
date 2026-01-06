package proto

// LogLinePayload encodes a MsgLogLine payload.
//
// Convention:
// - Payload is UTF-8 bytes without a trailing newline.
// - Delivery is best-effort; callers may drop on overflow.
func LogLinePayload(b []byte) []byte {
	if b == nil {
		return nil
	}
	cp := make([]byte, len(b))
	copy(cp, b)
	return cp
}
