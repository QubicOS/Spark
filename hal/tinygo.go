//go:build tinygo && baremetal

package hal

import (
	"machine"
	"time"
)

type tinyGoHAL struct {
	logger *uartLogger
	led    *pinLED
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

	return &tinyGoHAL{
		logger: &uartLogger{uart: uart},
		led:    &pinLED{pin: ledPin},
		fb:     &stubFramebuffer{w: 320, h: 320, format: PixelFormatRGB565},
		kbd:    &stubKeyboard{},
		t:      newTinyGoTime(),
		flash:  stubFlash{},
		net:    nullNetwork{},
		audio:  newTinyGoAudio(),
	}
}

func (h *tinyGoHAL) Logger() Logger   { return h.logger }
func (h *tinyGoHAL) LED() LED         { return h.led }
func (h *tinyGoHAL) Display() Display { return tinyGoDisplay{fb: h.fb} }
func (h *tinyGoHAL) Input() Input     { return tinyGoInput{kbd: h.kbd} }
func (h *tinyGoHAL) Flash() Flash     { return h.flash }
func (h *tinyGoHAL) Time() Time       { return h.t }
func (h *tinyGoHAL) Network() Network { return h.net }
func (h *tinyGoHAL) Audio() Audio     { return h.audio }

type tinyGoDisplay struct {
	fb Framebuffer
}

func (d tinyGoDisplay) Framebuffer() Framebuffer { return d.fb }

type tinyGoInput struct {
	kbd Keyboard
}

func (in tinyGoInput) Keyboard() Keyboard { return in.kbd }

type tinyGoTime struct {
	ch  chan uint64
	seq uint64
}

func newTinyGoTime() *tinyGoTime {
	t := &tinyGoTime{ch: make(chan uint64, 16)}
	go func() {
		ticker := time.NewTicker(1 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			t.seq++
			select {
			case t.ch <- t.seq:
			default:
			}
		}
	}()
	return t
}

func (t *tinyGoTime) Ticks() <-chan uint64 { return t.ch }

type uartLogger struct {
	uart *machine.UART
}

func (l *uartLogger) WriteLineString(s string) {
	for i := 0; i < len(s); i++ {
		l.uart.WriteByte(s[i])
	}
	l.uart.WriteByte('\r')
	l.uart.WriteByte('\n')
}

func (l *uartLogger) WriteLineBytes(b []byte) {
	for i := 0; i < len(b); i++ {
		l.uart.WriteByte(b[i])
	}
	l.uart.WriteByte('\r')
	l.uart.WriteByte('\n')
}

type pinLED struct {
	pin machine.Pin
}

func (l *pinLED) High() { l.pin.High() }
func (l *pinLED) Low()  { l.pin.Low() }
