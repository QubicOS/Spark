//go:build tinygo && baremetal && !picocalc

package hal

import (
	"machine"
)

type tinyGoHAL struct {
	logger *uartLogger
	led    *pinLED
	gpio   GPIO
	fb     Framebuffer
	kbd    Keyboard
	t      *tinyGoTime
	flash  Flash
	net    Network
	audio  Audio
}

// New returns a Pico 2 (RP2350) HAL implementation.
//
// UART: UART0 on GP0 (TX) / GP1 (RX), 115200 8N1.
func New() HAL {
	uart := machine.UART0
	uart.Configure(machine.UARTConfig{
		BaudRate: 115200,
		TX:       machine.GP0,
		RX:       machine.GP1,
	})

	ledPin := machine.LED
	ledPin.Configure(machine.PinConfig{Mode: machine.PinOutput})

	led := &pinLED{pin: ledPin}
	return &tinyGoHAL{
		logger: &uartLogger{uart: uart},
		led:    led,
		gpio:   newVirtualGPIO([]GPIOPin{newLEDPin("LED", led)}),
		fb:     &stubFramebuffer{w: 320, h: 320, format: PixelFormatRGB565},
		kbd:    &stubKeyboard{},
		t:      newTinyGoTime(),
		flash:  newRP2Flash(),
		net:    nullNetwork{},
		audio:  newTinyGoAudio(),
	}
}

func (h *tinyGoHAL) Logger() Logger   { return h.logger }
func (h *tinyGoHAL) LED() LED         { return h.led }
func (h *tinyGoHAL) GPIO() GPIO       { return h.gpio }
func (h *tinyGoHAL) Display() Display { return tinyGoDisplay{fb: h.fb} }
func (h *tinyGoHAL) Input() Input     { return tinyGoInput{kbd: h.kbd} }
func (h *tinyGoHAL) Flash() Flash     { return h.flash }
func (h *tinyGoHAL) Time() Time       { return h.t }
func (h *tinyGoHAL) Network() Network { return h.net }
func (h *tinyGoHAL) Audio() Audio     { return h.audio }
