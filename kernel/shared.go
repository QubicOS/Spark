package kernel

import "sync/atomic"

// SharedBuffer is a simple shared-memory region for "hybrid" IPC experiments.
//
// In v0 there is no memory protection: all tasks can access it.
type SharedBuffer struct {
	seq atomic.Uint32
	buf [MaxMessageBytes]byte
	n   atomic.Uint32
}

// Write copies data into the buffer and bumps the sequence counter.
func (b *SharedBuffer) Write(data []byte) uint32 {
	count := uint32(len(data))
	if count > MaxMessageBytes {
		count = MaxMessageBytes
	}

	copy(b.buf[:count], data[:count])
	b.n.Store(count)
	return b.seq.Add(1)
}

// Read returns the last written data and the current sequence number.
func (b *SharedBuffer) Read(dst []byte) (seq uint32, count int) {
	seq = b.seq.Load()
	n := b.n.Load()
	if n > uint32(len(dst)) {
		n = uint32(len(dst))
	}
	copy(dst[:n], b.buf[:n])
	return seq, int(n)
}
