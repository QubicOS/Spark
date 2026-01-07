package tea

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestParseHeader(t *testing.T) {
	data := make([]byte, 32)
	binary.LittleEndian.PutUint32(data[0:4], Magic)
	binary.LittleEndian.PutUint16(data[4:6], 44100)
	data[6] = 1 // Channels
	data[7] = CodecPCM16
	binary.LittleEndian.PutUint16(data[8:10], 100)
	binary.LittleEndian.PutUint16(data[10:12], 200)
	binary.LittleEndian.PutUint32(data[12:16], 1000)

	h, err := ParseHeader(data)
	if err != nil {
		t.Fatalf("ParseHeader failed: %v", err)
	}

	if h.SampleRate != 44100 {
		t.Errorf("Expected SampleRate 44100, got %d", h.SampleRate)
	}
	if h.CodecID != CodecPCM16 {
		t.Errorf("Expected CodecID %d, got %d", CodecPCM16, h.CodecID)
	}
}

func TestDecodePCM16(t *testing.T) {
	// Create a mock PCM16 block
	// Header
	header := make([]byte, 32)
	binary.LittleEndian.PutUint32(header[0:4], Magic)
	binary.LittleEndian.PutUint16(header[4:6], 44100)
	header[6] = 1
	header[7] = CodecPCM16
	binary.LittleEndian.PutUint16(header[8:10], 4)  // 4 samples
	binary.LittleEndian.PutUint16(header[10:12], 8) // 8 bytes
	binary.LittleEndian.PutUint32(header[12:16], 4) // total samples

	// Data: 4 samples * 2 bytes = 8 bytes
	block := []byte{
		0x00, 0x00, // 0
		0xFF, 0x7F, // 32767
		0x00, 0x80, // -32768
		0x01, 0x00, // 1
	}

	buf := bytes.NewReader(append(header, block...))

	dec, err := NewDecoder(buf)
	if err != nil {
		t.Fatalf("NewDecoder failed: %v", err)
	}

	out := make([]int16, 4)
	n, err := dec.DecodeBlock(out)
	if err != nil {
		t.Fatalf("DecodeBlock failed: %v", err)
	}

	if n != 4 {
		t.Errorf("Expected 4 samples, got %d", n)
	}

	expected := []int16{0, 32767, -32768, 1}
	for i, v := range expected {
		if out[i] != v {
			t.Errorf("Sample %d: expected %d, got %d", i, v, out[i])
		}
	}
}

func TestDecodeIMAADPCMBlock(t *testing.T) {
	// Block: predictor=0, index=0, then 3 samples.
	// Use nibble 0 (diff=step>>3=0) so predictor stays 0.
	block := []byte{
		0x00, 0x00, // predictor
		0x00, // index
		0x00, // nibbles: 0,0
		0x00, // nibble: 0,0 (we only need one more)
	}
	out := make([]int16, 4)
	n, err := DecodeIMAADPCMBlock(block, 4, out)
	if err != nil {
		t.Fatalf("DecodeIMAADPCMBlock failed: %v", err)
	}
	if n != 4 {
		t.Fatalf("expected 4 samples, got %d", n)
	}
	for i := 0; i < 4; i++ {
		if out[i] != 0 {
			t.Fatalf("sample %d expected 0 got %d", i, out[i])
		}
	}
}

func TestEncodeDecodeIMAADPCMBlockRoundTripZero(t *testing.T) {
	samples := []int16{0, 0, 0, 0, 0, 0, 0, 0}
	spb := len(samples)
	block := make([]byte, 3+(spb-1+1)/2)
	if err := EncodeIMAADPCMBlock(samples, spb, block); err != nil {
		t.Fatalf("EncodeIMAADPCMBlock: %v", err)
	}
	out := make([]int16, spb)
	n, err := DecodeIMAADPCMBlock(block, spb, out)
	if err != nil {
		t.Fatalf("DecodeIMAADPCMBlock: %v", err)
	}
	if n != spb {
		t.Fatalf("expected %d, got %d", spb, n)
	}
	for i := 0; i < spb; i++ {
		if out[i] != 0 {
			t.Fatalf("sample %d expected 0 got %d", i, out[i])
		}
	}
}
