//go:build tinygo && baremetal && picocalc

package hal

import (
	"machine"
	"time"
)

type picoCalcHAL struct {
	logger *uartLogger
	led    *pinLED
	gpio   GPIO
	fb     Framebuffer
	kbd    Keyboard
	t      *tinyGoTime
	flash  Flash
	net    Network
	audio  Audio
	serial Serial
}

// New returns a PicoCalc HAL implementation (Pico/Pico2 on the PicoCalc carrier).
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
	disp, err := newPicoCalcDisplay()
	if err != nil {
		disp = newPicoCalcDisplayStub()
	}

	var kbd Keyboard
	if kb, err := newPicoCalcKeyboard(); err == nil {
		kbd = kb
	} else {
		kbd = &stubKeyboard{}
	}

	return &picoCalcHAL{
		logger: &uartLogger{uart: uart},
		led:    led,
		gpio:   newVirtualGPIO([]GPIOPin{newLEDPin("LED", led)}),
		fb:     disp,
		kbd:    kbd,
		t:      newTinyGoTime(),
		flash:  newRP2Flash(),
		net:    nullNetwork{},
		audio:  newTinyGoAudio(),
		serial: &uartSerial{uart: uart},
	}
}

func (h *picoCalcHAL) Logger() Logger   { return h.logger }
func (h *picoCalcHAL) LED() LED         { return h.led }
func (h *picoCalcHAL) GPIO() GPIO       { return h.gpio }
func (h *picoCalcHAL) Display() Display { return tinyGoDisplay{fb: h.fb} }
func (h *picoCalcHAL) Input() Input     { return tinyGoInput{kbd: h.kbd} }
func (h *picoCalcHAL) Flash() Flash     { return h.flash }
func (h *picoCalcHAL) Time() Time       { return h.t }
func (h *picoCalcHAL) Network() Network { return h.net }
func (h *picoCalcHAL) Audio() Audio     { return h.audio }
func (h *picoCalcHAL) Serial() Serial   { return h.serial }

type picoCalcFramebuffer struct {
	w      int
	h      int
	stride int
	buf    []byte

	lcd *ili9488
}

func (f *picoCalcFramebuffer) Width() int          { return f.w }
func (f *picoCalcFramebuffer) Height() int         { return f.h }
func (f *picoCalcFramebuffer) Format() PixelFormat { return PixelFormatRGB565 }
func (f *picoCalcFramebuffer) StrideBytes() int    { return f.stride }
func (f *picoCalcFramebuffer) Buffer() []byte      { return f.buf }

func (f *picoCalcFramebuffer) ClearRGB(r, g, b uint8) {
	pixel := rgb565(r, g, b)
	lo := byte(pixel)
	hi := byte(pixel >> 8)
	for i := 0; i < len(f.buf); i += 2 {
		f.buf[i] = lo
		f.buf[i+1] = hi
	}
}

func (f *picoCalcFramebuffer) Present() error {
	if f.lcd == nil {
		return ErrNotImplemented
	}
	return f.lcd.blitRGB565LittleEndian(f.buf, f.w, f.h)
}

func newPicoCalcDisplay() (*picoCalcFramebuffer, error) {
	lcd, err := initILI9488()
	if err != nil {
		return nil, err
	}

	const w = 320
	const h = 320
	return &picoCalcFramebuffer{
		w:      w,
		h:      h,
		stride: w * 2,
		buf:    make([]byte, w*h*2),
		lcd:    lcd,
	}, nil
}

func newPicoCalcDisplayStub() *picoCalcFramebuffer {
	const w = 320
	const h = 320
	return &picoCalcFramebuffer{
		w:      w,
		h:      h,
		stride: w * 2,
		buf:    make([]byte, w*h*2),
	}
}

type picoCalcKeyboard struct {
	ch chan KeyEvent
}

func (k *picoCalcKeyboard) Events() <-chan KeyEvent { return k.ch }

func newPicoCalcKeyboard() (*picoCalcKeyboard, error) {
	dev := &picoCalcKeyboard{ch: make(chan KeyEvent, 64)}
	kbd, err := initI2CKeyboard()
	if err != nil {
		return nil, err
	}

	go func() {
		defer close(dev.ch)
		for {
			ev, ok := kbd.readEvent()
			if ok {
				select {
				case dev.ch <- ev:
				default:
				}
			}
			time.Sleep(2 * time.Millisecond)
		}
	}()

	return dev, nil
}
