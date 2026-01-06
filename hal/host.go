//go:build !tinygo

package hal

import (
	"fmt"
	"os"
	"sync"
)

type hostHAL struct {
	logger *hostLogger
	led    *hostLED
	fb     *hostFramebuffer
	kbd    *hostKeyboard
	t      *hostTime
	flash  *hostFlash
	net    Network
}

// New returns a host HAL implementation.
func New() HAL {
	logger := &hostLogger{w: os.Stdout}
	t := newHostTime()
	return &hostHAL{
		logger: logger,
		led:    &hostLED{logger: logger},
		fb:     newHostFramebuffer(320, 320),
		kbd:    newHostKeyboard(),
		t:      t,
		flash:  newHostFlash(),
		net:    nullNetwork{},
	}
}

func (h *hostHAL) Logger() Logger   { return h.logger }
func (h *hostHAL) LED() LED         { return h.led }
func (h *hostHAL) Display() Display { return hostDisplay{fb: h.fb} }
func (h *hostHAL) Input() Input     { return hostInput{kbd: h.kbd} }
func (h *hostHAL) Flash() Flash     { return h.flash }
func (h *hostHAL) Time() Time       { return h.t }
func (h *hostHAL) Network() Network { return h.net }

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
