package tea

import (
	"encoding/binary"
	"fmt"
	"io"
)

// WriteHeader writes a 32-byte TEA header.
func WriteHeader(w io.Writer, h Header) error {
	if err := h.Validate(); err != nil {
		return fmt.Errorf("tea write header: %w", err)
	}

	var b [32]byte
	binary.LittleEndian.PutUint32(b[0:4], h.Magic)
	binary.LittleEndian.PutUint16(b[4:6], h.SampleRate)
	b[6] = h.Channels
	b[7] = h.CodecID
	binary.LittleEndian.PutUint16(b[8:10], h.SamplesPerBlock)
	binary.LittleEndian.PutUint16(b[10:12], h.BlockSize)
	binary.LittleEndian.PutUint32(b[12:16], h.TotalSamples)
	binary.LittleEndian.PutUint16(b[16:18], h.Flags)
	copy(b[18:32], h.Reserved[:])

	_, err := w.Write(b[:])
	if err != nil {
		return fmt.Errorf("tea write header: %w", err)
	}
	return nil
}

// EncodePCM16Block encodes up to samplesPerBlock samples into dst.
//
// dst must be of size samplesPerBlock*2.
func EncodePCM16Block(samples []int16, samplesPerBlock int, dst []byte) error {
	if len(dst) != samplesPerBlock*2 {
		return fmt.Errorf("tea pcm16: bad dst size")
	}
	for i := 0; i < samplesPerBlock; i++ {
		var v int16
		if i < len(samples) {
			v = samples[i]
		}
		binary.LittleEndian.PutUint16(dst[i*2:i*2+2], uint16(v))
	}
	return nil
}
