//go:build tinygo && baremetal

package hal

import (
	"machine"
	"time"
)

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

type uartSerial struct {
	uart *machine.UART
}

func (s *uartSerial) Read(p []byte) (int, error) {
	if s.uart == nil {
		return 0, ErrNotImplemented
	}
	return s.uart.Read(p)
}

func (s *uartSerial) Write(p []byte) (int, error) {
	if s.uart == nil {
		return 0, ErrNotImplemented
	}
	return s.uart.Write(p)
}
