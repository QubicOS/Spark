package tea

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	// MaxBlockBytes is the maximum supported TEA block size for realtime decoding.
	MaxBlockBytes = 2048
)

var (
	errOutTooSmall   = errors.New("tea: output buffer too small")
	errShortBlock    = errors.New("tea: short block")
	errUnsupported   = errors.New("tea: unsupported")
	errHeaderInvalid = errors.New("tea: invalid header")
)

// Decoder reads TEA blocks and decodes them to PCM16.
//
// The hot-path DecodeBlock performs no heap allocations.
type Decoder struct {
	r io.ReadSeeker

	Header Header

	samplePos uint32

	blockBuf [MaxBlockBytes]byte
}

// NewDecoder creates a decoder and reads the TEA header.
func NewDecoder(r io.ReadSeeker) (*Decoder, error) {
	if r == nil {
		return nil, fmt.Errorf("tea decoder: nil reader")
	}

	var hdrRaw [32]byte
	if _, err := io.ReadFull(r, hdrRaw[:]); err != nil {
		return nil, fmt.Errorf("tea decoder: read header: %w", err)
	}
	h, err := ParseHeader(hdrRaw[:])
	if err != nil {
		return nil, fmt.Errorf("tea decoder: %w", err)
	}
	if uint32(h.BlockSize) > MaxBlockBytes {
		return nil, fmt.Errorf("tea decoder: block too large: %d > %d", h.BlockSize, MaxBlockBytes)
	}

	return &Decoder{r: r, Header: *h}, nil
}

// SeekToBlock seeks to the specified block index (0-based).
func (d *Decoder) SeekToBlock(blockIndex uint32) error {
	if d == nil {
		return errors.New("tea decoder: nil")
	}

	off := int64(32) + int64(blockIndex)*int64(d.Header.BlockSize)
	if _, err := d.r.Seek(off, io.SeekStart); err != nil {
		return fmt.Errorf("tea decoder: seek: %w", err)
	}

	spb := uint32(d.Header.SamplesPerBlock)
	d.samplePos = blockIndex * spb
	if d.samplePos > d.Header.TotalSamples {
		d.samplePos = d.Header.TotalSamples
	}
	return nil
}

// DecodeBlock reads and decodes the next block into out.
//
// It returns the number of decoded samples. At end of stream it returns io.EOF.
func (d *Decoder) DecodeBlock(out []int16) (int, error) {
	if d == nil {
		return 0, errors.New("tea decoder: nil")
	}
	if err := d.Header.Validate(); err != nil {
		return 0, fmt.Errorf("%w: %v", errHeaderInvalid, err)
	}

	if d.samplePos >= d.Header.TotalSamples {
		return 0, io.EOF
	}

	spb := int(d.Header.SamplesPerBlock)
	if len(out) < spb {
		return 0, errOutTooSmall
	}

	block := d.blockBuf[:d.Header.BlockSize]
	if _, err := io.ReadFull(d.r, block); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return 0, io.EOF
		}
		return 0, fmt.Errorf("tea decoder: read block: %w", err)
	}

	want := spb
	remain := int(d.Header.TotalSamples - d.samplePos)
	if remain < want {
		want = remain
	}

	var n int
	switch d.Header.CodecID {
	case CodecPCM16:
		n = decodePCM16Block(block, out, want)
	case CodecIMAADPCM:
		decoded, err := DecodeIMAADPCMBlock(block, spb, out)
		if err != nil {
			return 0, err
		}
		n = decoded
		if n > want {
			n = want
		}
	default:
		return 0, errUnsupported
	}

	if n < want {
		return 0, errShortBlock
	}

	d.samplePos += uint32(n)
	return n, nil
}

func decodePCM16Block(block []byte, out []int16, want int) int {
	max := len(block) / 2
	if want < max {
		max = want
	}
	for i := 0; i < max; i++ {
		out[i] = int16(binary.LittleEndian.Uint16(block[i*2 : i*2+2]))
	}
	return max
}
