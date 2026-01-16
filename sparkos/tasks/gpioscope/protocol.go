package gpioscope

import (
	"fmt"
	"strings"
)

func decodeUART(samples []uint32, rxPin int, baud int, periodTicks uint32) []string {
	if rxPin < 0 || rxPin >= 32 {
		return []string{"UART: set RX (Shift+R) on a pin"}
	}
	if baud <= 0 {
		return []string{"UART: invalid baud"}
	}
	if periodTicks == 0 {
		return []string{"UART: invalid sample period"}
	}

	sampleRate := 1000.0 / float64(periodTicks)
	samplesPerBit := sampleRate / float64(baud)
	if samplesPerBit < 2 {
		return []string{fmt.Sprintf("UART: need higher sample rate (samples/bit=%.2f)", samplesPerBit)}
	}
	spb := int(samplesPerBit + 0.5)
	if spb < 1 {
		spb = 1
	}

	var out []string
	for i := 1; i+spb*10 < len(samples); i++ {
		prev := (samples[i-1]>>uint(rxPin))&1 != 0
		cur := (samples[i]>>uint(rxPin))&1 != 0
		if prev || !cur {
			continue
		}

		start := i
		var b uint8
		for bit := 0; bit < 8; bit++ {
			si := start + spb*(bit+1) + spb/2
			if si >= len(samples) {
				break
			}
			val := (samples[si]>>uint(rxPin))&1 != 0
			if val {
				b |= 1 << uint(bit)
			}
		}
		out = append(out, fmt.Sprintf("UART: 0x%02X '%s'", b, safeASCII(b)))
		i = start + spb*10
		if len(out) >= 128 {
			break
		}
	}
	if len(out) == 0 {
		return []string{"UART: no frames"}
	}
	return out
}

func safeASCII(b byte) string {
	if b >= 0x20 && b <= 0x7e {
		return string([]byte{b})
	}
	return "."
}

func decodeSPI(samples []uint32, clk, mosi, miso, cs int, cpol, cpha bool) []string {
	if clk < 0 || clk >= 32 {
		return []string{"SPI: set CLK (Shift+C) on a pin"}
	}
	if cs < 0 || cs >= 32 {
		return []string{"SPI: set CS (Shift+S) on a pin"}
	}

	var out []string
	var curByteMOSI uint8
	var curByteMISO uint8
	bitPos := 0

	clkIdle := boolFromBit(samples[0], clk)
	_ = clkIdle

	lastCLK := boolFromBit(samples[0], clk)
	for i := 1; i < len(samples); i++ {
		if boolFromBit(samples[i], cs) {
			bitPos = 0
			curByteMOSI = 0
			curByteMISO = 0
			lastCLK = boolFromBit(samples[i], clk)
			continue
		}

		clkNow := boolFromBit(samples[i], clk)
		edge := clkNow != lastCLK
		if !edge {
			continue
		}

		sampleOnEdge := (lastCLK == cpol) != cpha
		if sampleOnEdge {
			if mosi >= 0 && mosi < 32 && boolFromBit(samples[i], mosi) {
				curByteMOSI |= 1 << uint(7-bitPos)
			}
			if miso >= 0 && miso < 32 && boolFromBit(samples[i], miso) {
				curByteMISO |= 1 << uint(7-bitPos)
			}
			bitPos++
			if bitPos == 8 {
				out = append(out, fmt.Sprintf("SPI: MOSI=0x%02X MISO=0x%02X", curByteMOSI, curByteMISO))
				bitPos = 0
				curByteMOSI = 0
				curByteMISO = 0
				if len(out) >= 128 {
					break
				}
			}
		}
		lastCLK = clkNow
	}
	if len(out) == 0 {
		return []string{"SPI: no data"}
	}
	return out
}

func decodeI2C(samples []uint32, scl, sda int) []string {
	if scl < 0 || scl >= 32 {
		return []string{"I2C: set SCL (Shift+A) on a pin"}
	}
	if sda < 0 || sda >= 32 {
		return []string{"I2C: set SDA (Shift+D) on a pin"}
	}

	var out []string
	inFrame := false
	lastSCL := boolFromBit(samples[0], scl)
	lastSDA := boolFromBit(samples[0], sda)
	var curByte uint8
	bitPos := 0

	flushByte := func(ack bool) {
		if bitPos != 8 {
			return
		}
		suffix := "NACK"
		if ack {
			suffix = "ACK"
		}
		out = append(out, fmt.Sprintf("I2C: 0x%02X %s", curByte, suffix))
		curByte = 0
		bitPos = 0
	}

	for i := 1; i < len(samples); i++ {
		sclNow := boolFromBit(samples[i], scl)
		sdaNow := boolFromBit(samples[i], sda)

		if lastSDA && !sdaNow && sclNow {
			inFrame = true
			out = append(out, "I2C: START")
			curByte = 0
			bitPos = 0
		}
		if !lastSDA && sdaNow && sclNow && inFrame {
			out = append(out, "I2C: STOP")
			inFrame = false
			curByte = 0
			bitPos = 0
		}

		if inFrame && !lastSCL && sclNow {
			if bitPos < 8 {
				if sdaNow {
					curByte |= 1 << uint(7-bitPos)
				}
				bitPos++
			} else {
				flushByte(!sdaNow)
			}
		}

		lastSCL = sclNow
		lastSDA = sdaNow
		if len(out) >= 128 {
			break
		}
	}
	if len(out) == 0 {
		return []string{"I2C: no activity"}
	}
	return compact(out)
}

func boolFromBit(v uint32, pin int) bool {
	return v&(1<<uint(pin)) != 0
}

func compact(lines []string) []string {
	var out []string
	for _, s := range lines {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	return out
}
