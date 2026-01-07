package tea

import "errors"

// IMA ADPCM step table.
var imaStepTable = [...]int{
	7, 8, 9, 10, 11, 12, 13, 14, 16, 17,
	19, 21, 23, 25, 28, 31, 34, 37, 41, 45,
	50, 55, 60, 66, 73, 80, 88, 97, 107, 118,
	130, 143, 157, 173, 190, 209, 230, 253, 279, 307,
	337, 371, 408, 449, 494, 544, 598, 658, 724, 796,
	876, 963, 1060, 1166, 1282, 1411, 1552, 1707, 1878, 2066,
	2272, 2499, 2749, 3024, 3327, 3660, 4026, 4428, 4871, 5358,
	5894, 6484, 7132, 7845, 8630, 9493, 10442, 11487, 12635, 13899,
	15289, 16818, 18500, 20350, 22385, 24623, 27086, 29794, 32767,
}

// IMA ADPCM index table.
var imaIndexTable = [...]int{
	-1, -1, -1, -1, 2, 4, 6, 8,
	-1, -1, -1, -1, 2, 4, 6, 8,
}

var errBadADPCMBlock = errors.New("ima-adpcm: bad block")

func clampI16(x int) int16 {
	if x > 32767 {
		return 32767
	}
	if x < -32768 {
		return -32768
	}
	return int16(x)
}

func clampIndex(i int) int {
	if i < 0 {
		return 0
	}
	if i > 88 {
		return 88
	}
	return i
}

// EncodeIMAADPCMBlock encodes samples into a TEA IMA-ADPCM block.
//
// samplesPerBlock controls the produced block size; if samples has fewer samples,
// the remainder is padded with the last sample (or 0 if empty).
//
// dst must have exact size 3+ceil((samplesPerBlock-1)/2).
func EncodeIMAADPCMBlock(samples []int16, samplesPerBlock int, dst []byte) error {
	if samplesPerBlock <= 0 {
		return errBadADPCMBlock
	}
	want := 3 + (samplesPerBlock-1+1)/2
	if len(dst) != want {
		return errBadADPCMBlock
	}

	var predictor int16
	if len(samples) > 0 {
		predictor = samples[0]
	}
	dst[0] = byte(uint16(predictor))
	dst[1] = byte(uint16(predictor) >> 8)

	stepIndex := 0
	dst[2] = byte(stepIndex)

	need := samplesPerBlock - 1
	out := dst[3:]

	curPred := predictor
	curIdx := stepIndex

	lastSample := predictor
	if len(samples) > 0 {
		lastSample = samples[len(samples)-1]
	}

	sampleAt := func(i int) int16 {
		if i < len(samples) {
			return samples[i]
		}
		return lastSample
	}

	bytePos := 0
	low := true
	var cur byte

	for i := 1; i <= need; i++ {
		target := sampleAt(i)
		nibble, nextPred, nextIdx := imaEncodeNibble(curPred, curIdx, target)
		curPred, curIdx = nextPred, nextIdx

		if low {
			cur = byte(nibble & 0x0F)
			low = false
		} else {
			cur |= byte((nibble & 0x0F) << 4)
			if bytePos >= len(out) {
				return errBadADPCMBlock
			}
			out[bytePos] = cur
			bytePos++
			low = true
			cur = 0
		}
	}

	if !low {
		if bytePos >= len(out) {
			return errBadADPCMBlock
		}
		out[bytePos] = cur
	}

	return nil
}

func imaEncodeNibble(predictor int16, stepIndex int, sample int16) (nibble uint8, nextPred int16, nextIdx int) {
	stepIndex = clampIndex(stepIndex)
	step := imaStepTable[stepIndex]

	diff := int(sample) - int(predictor)
	sign := 0
	if diff < 0 {
		sign = 8
		diff = -diff
	}

	code := 0
	vpdiff := step >> 3

	if diff >= step {
		code |= 4
		diff -= step
		vpdiff += step
	}
	if diff >= (step >> 1) {
		code |= 2
		diff -= step >> 1
		vpdiff += step >> 1
	}
	if diff >= (step >> 2) {
		code |= 1
		vpdiff += step >> 2
	}

	pred := int(predictor)
	if sign != 0 {
		pred -= vpdiff
	} else {
		pred += vpdiff
	}
	nextPred = clampI16(pred)

	nextIdx = clampIndex(stepIndex + imaIndexTable[code|sign])
	return uint8(code | sign), nextPred, nextIdx
}

// DecodeIMAADPCMBlock decodes a single TEA IMA-ADPCM block into out.
//
// Block layout (TEA v1.0):
//   - int16 predictor (LE)
//   - uint8 step_index
//   - packed ADPCM nibbles for (samplesPerBlock-1) samples, low nibble first.
//
// out must have capacity for at least samplesPerBlock samples.
func DecodeIMAADPCMBlock(block []byte, samplesPerBlock int, out []int16) (int, error) {
	if samplesPerBlock <= 0 {
		return 0, errBadADPCMBlock
	}
	if len(out) < samplesPerBlock {
		return 0, errBadADPCMBlock
	}
	if len(block) < 3 {
		return 0, errBadADPCMBlock
	}

	predictor := int16(uint16(block[0]) | uint16(block[1])<<8)
	stepIndex := clampIndex(int(block[2]))

	out[0] = predictor
	written := 1

	needNibbles := samplesPerBlock - 1
	data := block[3:]
	for i := 0; i < len(data) && needNibbles > 0; i++ {
		b := data[i]
		nibs := [2]uint8{b & 0x0F, (b >> 4) & 0x0F}
		for j := 0; j < 2 && needNibbles > 0; j++ {
			n := int(nibs[j])
			step := imaStepTable[stepIndex]
			diff := step >> 3
			if (n & 4) != 0 {
				diff += step
			}
			if (n & 2) != 0 {
				diff += step >> 1
			}
			if (n & 1) != 0 {
				diff += step >> 2
			}
			pred := int(predictor)
			if (n & 8) != 0 {
				pred -= diff
			} else {
				pred += diff
			}
			predictor = clampI16(pred)

			stepIndex = clampIndex(stepIndex + imaIndexTable[n])

			out[written] = predictor
			written++
			needNibbles--
		}
	}

	if needNibbles != 0 {
		return 0, errBadADPCMBlock
	}
	return written, nil
}
