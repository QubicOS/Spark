//go:build tinygo && baremetal && picocalc

package hal

import (
	"errors"
	"machine"
	"time"
)

type ili9488 struct {
	spi machine.SPI
	cs  machine.Pin
	dc  machine.Pin
	rst machine.Pin

	txBuf []byte
}

func initILI9488() (*ili9488, error) {
	if machine.SPI1 == nil {
		return nil, errors.New("SPI1 unavailable")
	}

	machine.SPI1.Configure(machine.SPIConfig{
		SCK:       machine.GP10,
		SDO:       machine.GP11,
		SDI:       machine.GP12,
		Frequency: 40_000_000,
	})

	lcd := &ili9488{
		spi:   *machine.SPI1,
		cs:    machine.GP13,
		dc:    machine.GP14,
		rst:   machine.GP15,
		txBuf: make([]byte, 4096),
	}

	lcd.cs.Configure(machine.PinConfig{Mode: machine.PinOutput})
	lcd.dc.Configure(machine.PinConfig{Mode: machine.PinOutput})
	lcd.rst.Configure(machine.PinConfig{Mode: machine.PinOutput})
	lcd.cs.High()
	lcd.dc.High()
	lcd.rst.High()

	lcd.reset()
	lcd.init()

	return lcd, nil
}

func (d *ili9488) reset() {
	d.rst.Low()
	time.Sleep(64 * time.Millisecond)
	d.rst.High()
	time.Sleep(140 * time.Millisecond)
}

func (d *ili9488) init() {
	// Power control.
	d.cmd(0xC0, 0x17, 0x15) // PWCTRL1
	d.cmd(0xC1, 0x41)       // PWCTRL2

	// VCOM control.
	d.cmd(0xC5, 0x00, 0x12, 0x80, 0x40) // VMCTRL

	// Pixel format: 16bpp.
	d.cmd(0x3A, 0x55) // COLMOD

	// Frame rate / display function.
	d.cmd(0xB1, 0xA0, 0x11)       // FRMCTRL1
	d.cmd(0xB6, 0x02, 0x22, 0x27) // DISCTRL (320 lines)

	// Inversion mode. Many panels look correct with inversion enabled.
	d.cmd(0x21) // INVON

	// Memory access control: mirror for PicoCalc wiring + BGR panel order.
	d.cmd(0x36, 0x40|0x04|0x08) // MX|MH|BGR

	d.cmd(0x11) // SLPOUT
	time.Sleep(120 * time.Millisecond)
	d.cmd(0x29) // DISPON
}

func (d *ili9488) cmd(cmd byte, data ...byte) {
	d.cs.Low()
	d.dc.Low()
	d.spi.Tx([]byte{cmd}, nil)
	d.dc.High()
	if len(data) > 0 {
		d.spi.Tx(data, nil)
	}
	d.cs.High()
}

func (d *ili9488) setWindow(x0, y0, x1, y1 uint16) {
	d.cmd(
		0x2A,
		byte(x0>>8), byte(x0),
		byte(x1>>8), byte(x1),
	)
	d.cmd(
		0x2B,
		byte(y0>>8), byte(y0),
		byte(y1>>8), byte(y1),
	)
	d.cmd(0x2C)
}

func (d *ili9488) blitRGB565LittleEndian(buf []byte, w, h int) error {
	if w <= 0 || h <= 0 || len(buf) < w*h*2 {
		return errors.New("invalid framebuffer")
	}

	d.setWindow(0, 0, uint16(w-1), uint16(h-1))

	d.cs.Low()
	d.dc.High()

	chunk := d.txBuf
	if len(chunk)%2 != 0 {
		chunk = chunk[:len(chunk)-1]
	}
	if len(chunk) < 2 {
		return errors.New("tx buffer too small")
	}

	for off := 0; off < w*h*2; {
		n := len(chunk)
		remain := w*h*2 - off
		if n > remain {
			n = remain
			n &^= 1
		}
		src := buf[off : off+n]

		for i := 0; i < n; i += 2 {
			// The OS stores RGB565 in little-endian. The LCD expects big-endian.
			chunk[i] = src[i+1]
			chunk[i+1] = src[i]
		}
		d.spi.Tx(chunk[:n], nil)
		off += n
	}

	d.cs.High()
	return nil
}
