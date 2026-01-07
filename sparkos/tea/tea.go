package tea

import (
	"encoding/binary"
	"errors"
)

// Magic bytes "TEA1"
const Magic = 0x31414554 // "TEA1" in little-endian (T=0x54, E=0x45, A=0x41, 1=0x31)

// Codec IDs
const (
	CodecPCM16    = 0x01
	CodecIMAADPCM = 0x02
)

// Flags
const (
	FlagLoopEnabled = 1 << 0
	FlagHasEvents   = 1 << 1
)

// Header represents the fixed 32-byte TEA file header.
type Header struct {
	Magic           uint32
	SampleRate      uint16
	Channels        uint8
	CodecID         uint8
	SamplesPerBlock uint16
	BlockSize       uint16
	TotalSamples    uint32
	Flags           uint16
	Reserved        [14]byte
}

// ParseHeader reads the header from a byte slice.
func ParseHeader(data []byte) (*Header, error) {
	if len(data) < 32 {
		return nil, errors.New("header too short")
	}

	h := &Header{}
	h.Magic = binary.LittleEndian.Uint32(data[0:4])
	if h.Magic != Magic {
		return nil, errors.New("invalid magic")
	}

	h.SampleRate = binary.LittleEndian.Uint16(data[4:6])
	h.Channels = data[6]
	h.CodecID = data[7]
	h.SamplesPerBlock = binary.LittleEndian.Uint16(data[8:10])
	h.BlockSize = binary.LittleEndian.Uint16(data[10:12])
	h.TotalSamples = binary.LittleEndian.Uint32(data[12:16])
	h.Flags = binary.LittleEndian.Uint16(data[16:18])
	copy(h.Reserved[:], data[18:32])

	if h.Channels != 1 {
		return nil, errors.New("only mono supported in v1")
	}

	if err := h.Validate(); err != nil {
		return nil, err
	}
	return h, nil
}

// Validate checks header invariants required by TEA v1.0.
func (h *Header) Validate() error {
	if h.Magic != Magic {
		return errors.New("invalid magic")
	}
	if h.SampleRate == 0 {
		return errors.New("invalid sample rate")
	}
	if h.Channels != 1 {
		return errors.New("only mono supported in v1")
	}
	if h.SamplesPerBlock == 0 {
		return errors.New("invalid samples per block")
	}
	if h.BlockSize == 0 {
		return errors.New("invalid block size")
	}
	if h.TotalSamples == 0 {
		return errors.New("invalid total samples")
	}
	for i := range h.Reserved {
		if h.Reserved[i] != 0 {
			return errors.New("reserved must be 0")
		}
	}
	switch h.CodecID {
	case CodecPCM16:
		want := uint16(h.SamplesPerBlock) * 2
		if h.BlockSize != want {
			return errors.New("pcm16: block size must be samples_per_block*2")
		}
	case CodecIMAADPCM:
		// Layout: int16 predictor + int8 step_index + packed nibbles.
		// First sample is predictor; remaining (spb-1) samples are nibbles.
		spb := uint32(h.SamplesPerBlock)
		if spb < 1 {
			return errors.New("ima-adpcm: invalid samples per block")
		}
		nibbles := spb - 1
		bytes := (nibbles + 1) / 2
		want := uint32(2 + 1 + bytes)
		if uint32(h.BlockSize) != want {
			return errors.New("ima-adpcm: invalid block size")
		}
	default:
		return errors.New("unsupported codec")
	}
	return nil
}
