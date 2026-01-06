package proto

// AppControlPayload encodes a generic activate/deactivate command.
//
// Payload format:
//
//	b[0] == 0 => deactivate
//	b[0] != 0 => activate
func AppControlPayload(active bool) []byte {
	if active {
		return []byte{1}
	}
	return []byte{0}
}

func DecodeAppControlPayload(b []byte) (active bool, ok bool) {
	if len(b) != 1 {
		return false, false
	}
	return b[0] != 0, true
}
