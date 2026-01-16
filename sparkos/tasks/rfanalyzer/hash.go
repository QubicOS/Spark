package rfanalyzer

func fnv1a32(b []byte) uint32 {
	const (
		offset32 = 2166136261
		prime32  = 16777619
	)
	h := uint32(offset32)
	for _, v := range b {
		h ^= uint32(v)
		h *= prime32
	}
	if h == 0 {
		h = 1
	}
	return h
}
