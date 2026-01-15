package proto

import "encoding/binary"

type AudioState uint8

const (
	AudioStopped AudioState = iota
	AudioPlaying
	AudioPaused
)

// AudioSubscribePayload encodes a subscription request.
//
// The request payload is empty; the sender provides an endpoint capability in msg.Cap to receive MsgAudioStatus updates.
func AudioSubscribePayload() []byte { return nil }

// AudioPlayPayload encodes a play request.
//
// Layout:
//   - u8: flags (bit0=loop)
//   - bytes: UTF-8 path
func AudioPlayPayload(loop bool, path string) []byte {
	p := []byte(path)
	buf := make([]byte, 1+len(p))
	if loop {
		buf[0] = 1
	}
	copy(buf[1:], p)
	return buf
}

func DecodeAudioPlayPayload(b []byte) (loop bool, path string, ok bool) {
	if len(b) < 1 {
		return false, "", false
	}
	if b[0]&^byte(1) != 0 {
		return false, "", false
	}
	loop = (b[0] & 1) != 0
	return loop, string(b[1:]), true
}

// AudioSetVolumePayload encodes volume (0..255).
func AudioSetVolumePayload(vol uint8) []byte { return []byte{vol} }

func DecodeAudioSetVolumePayload(b []byte) (vol uint8, ok bool) {
	if len(b) != 1 {
		return 0, false
	}
	return b[0], true
}

// AudioStatusPayload encodes playback status.
//
// Layout (little-endian):
//   - u8: state (AudioState)
//   - u8: volume
//   - u16: sample rate
//   - u32: position samples
//   - u32: total samples
func AudioStatusPayload(state AudioState, volume uint8, sampleRate uint16, posSamples uint32, totalSamples uint32) []byte {
	buf := make([]byte, 12)
	buf[0] = uint8(state)
	buf[1] = volume
	binary.LittleEndian.PutUint16(buf[2:4], sampleRate)
	binary.LittleEndian.PutUint32(buf[4:8], posSamples)
	binary.LittleEndian.PutUint32(buf[8:12], totalSamples)
	return buf
}

func DecodeAudioStatusPayload(b []byte) (state AudioState, volume uint8, sampleRate uint16, posSamples uint32, totalSamples uint32, ok bool) {
	if len(b) != 12 {
		return 0, 0, 0, 0, 0, false
	}
	state = AudioState(b[0])
	volume = b[1]
	sampleRate = binary.LittleEndian.Uint16(b[2:4])
	posSamples = binary.LittleEndian.Uint32(b[4:8])
	totalSamples = binary.LittleEndian.Uint32(b[8:12])
	return state, volume, sampleRate, posSamples, totalSamples, true
}

// AudioMetersPayload encodes a simple equalizer meter update.
//
// Layout:
//   - u8: bands count
//   - u8[]: band levels (0..255)
func AudioMetersPayload(levels []uint8) []byte {
	if len(levels) > 32 {
		levels = levels[:32]
	}
	buf := make([]byte, 1+len(levels))
	buf[0] = uint8(len(levels))
	copy(buf[1:], levels)
	return buf
}

func DecodeAudioMetersPayload(b []byte) (levels []uint8, ok bool) {
	if len(b) < 1 {
		return nil, false
	}
	n := int(b[0])
	if n < 0 || n > 32 {
		return nil, false
	}
	if len(b) != 1+n {
		return nil, false
	}
	return b[1:], true
}
