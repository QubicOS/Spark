//go:build !tinygo

package hal

import (
	"fmt"
	"os"
	"sync"
	"time"
)

type hostHAL struct {
	logger *hostLogger
	led    *hostLED
	gpio   GPIO
	fb     *hostFramebuffer
	kbd    *hostKeyboard
	t      *hostTime
	flash  *hostFlash
	net    Network
	aud    Audio
	serial Serial
}

// New returns a host HAL implementation.
func New() HAL {
	logger := &hostLogger{w: os.Stdout}
	t := newHostTime()
	led := &hostLED{logger: logger}
	pins := []GPIOPin{newLEDPin("LED", led)}
	for i := 0; i < 7; i++ {
		pins = append(pins, newVirtualPin(fmt.Sprintf("GPIO%d", i+1), GPIOCapInput|GPIOCapOutput|GPIOCapPullUp|GPIOCapPullDown))
	}
	// Dummy signal sources for the GPIO scope app on host.
	pins = append(pins,
		newSignalPin("SIG1HZ", 1*time.Second, 500*time.Millisecond),
		newSignalPin("SIG5HZ", 200*time.Millisecond, 100*time.Millisecond),
		newSignalPin("SIGPULSE", 1*time.Second, 50*time.Millisecond),
		newSignalPin("SIGPWM25", 200*time.Millisecond, 50*time.Millisecond),
	)
	gpio := newVirtualGPIO(pins)
	return &hostHAL{
		logger: logger,
		led:    led,
		gpio:   gpio,
		fb:     newHostFramebuffer(320, 320),
		kbd:    newHostKeyboard(),
		t:      t,
		flash:  newHostFlash(),
		net:    nullNetwork{},
		aud:    newHostAudio(),
		serial: &hostSerial{r: os.Stdin, w: os.Stdout},
	}
}

func (h *hostHAL) Logger() Logger   { return h.logger }
func (h *hostHAL) LED() LED         { return h.led }
func (h *hostHAL) GPIO() GPIO       { return h.gpio }
func (h *hostHAL) Display() Display { return hostDisplay{fb: h.fb} }
func (h *hostHAL) Input() Input     { return hostInput{kbd: h.kbd} }
func (h *hostHAL) Flash() Flash     { return h.flash }
func (h *hostHAL) Time() Time       { return h.t }
func (h *hostHAL) Network() Network { return h.net }
func (h *hostHAL) Audio() Audio     { return h.aud }
func (h *hostHAL) Serial() Serial   { return h.serial }

type hostDisplay struct {
	fb *hostFramebuffer
}

func (d hostDisplay) Framebuffer() Framebuffer { return d.fb }

type hostInput struct {
	kbd *hostKeyboard
}

func (in hostInput) Keyboard() Keyboard { return in.kbd }

type hostLogger struct {
	mu sync.Mutex
	w  *os.File
}

func (l *hostLogger) WriteLineString(s string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Fprintln(l.w, s)
}

func (l *hostLogger) WriteLineBytes(b []byte) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.w.Write(b)
	l.w.Write([]byte{'\n'})
}

type hostLED struct {
	mu     sync.Mutex
	on     bool
	logger *hostLogger
}

func (l *hostLED) High() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.on = true
	l.logger.WriteLineString("led: HIGH")
}

func (l *hostLED) Low() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.on = false
	l.logger.WriteLineString("led: LOW")
}
