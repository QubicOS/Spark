//go:build tinygo && !baremetal

package hal

import (
	"fmt"
	"runtime"
	"time"
)

type tinyGoHostHAL struct {
	logger *tinyGoHostLogger
	led    *tinyGoHostLED
	fb     *tinyGoHostFramebuffer
	kbd    *tinyGoHostKeyboard
	t      *tinyGoHostTime
	flash  Flash
	net    Network
}

// New returns a TinyGo-on-host HAL implementation.
//
// This is used by `tinygo run` targets like linux/wasm where there is no MCU pin mapping.
func New() HAL {
	l := &tinyGoHostLogger{}
	return &tinyGoHostHAL{
		logger: l,
		led:    &tinyGoHostLED{logger: l},
		fb:     newTinyGoHostFramebuffer(320, 320),
		kbd:    newTinyGoHostKeyboard(),
		t:      newTinyGoHostTime(),
		flash:  stubFlash{},
		net:    nullNetwork{},
	}
}

func (h *tinyGoHostHAL) Logger() Logger   { return h.logger }
func (h *tinyGoHostHAL) LED() LED         { return h.led }
func (h *tinyGoHostHAL) Display() Display { return tinyGoHostDisplay{fb: h.fb} }
func (h *tinyGoHostHAL) Input() Input     { return tinyGoHostInput{kbd: h.kbd} }
func (h *tinyGoHostHAL) Flash() Flash     { return h.flash }
func (h *tinyGoHostHAL) Time() Time       { return h.t }
func (h *tinyGoHostHAL) Network() Network { return h.net }

type tinyGoHostDisplay struct {
	fb Framebuffer
}

func (d tinyGoHostDisplay) Framebuffer() Framebuffer { return d.fb }

type tinyGoHostInput struct {
	kbd Keyboard
}

func (in tinyGoHostInput) Keyboard() Keyboard { return in.kbd }

type tinyGoHostTime struct {
	ch  chan uint64
	seq uint64
}

func newTinyGoHostTime() *tinyGoHostTime {
	t := &tinyGoHostTime{ch: make(chan uint64, 16)}
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

func (t *tinyGoHostTime) Ticks() <-chan uint64 { return t.ch }

type tinyGoHostLogger struct{}

func (l *tinyGoHostLogger) WriteLineString(s string) {
	println(s)
}

func (l *tinyGoHostLogger) WriteLineBytes(b []byte) {
	println(string(b))
}

type tinyGoHostLED struct {
	on     bool
	logger *tinyGoHostLogger
}

func (l *tinyGoHostLED) High() {
	l.on = true
	l.logger.WriteLineString(fmt.Sprintf("led: HIGH (tinygo/%s)", runtime.GOOS))
}

func (l *tinyGoHostLED) Low() {
	l.on = false
	l.logger.WriteLineString(fmt.Sprintf("led: LOW (tinygo/%s)", runtime.GOOS))
}

type tinyGoHostKeyboard struct {
	ch chan KeyEvent
}

func newTinyGoHostKeyboard() *tinyGoHostKeyboard {
	return &tinyGoHostKeyboard{ch: make(chan KeyEvent)}
}

func (k *tinyGoHostKeyboard) Events() <-chan KeyEvent { return k.ch }
