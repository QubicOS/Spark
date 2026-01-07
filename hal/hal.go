package hal

import "errors"

// Logger writes newline-delimited log lines.
type Logger interface {
	WriteLineString(s string)
	WriteLineBytes(b []byte)
}

// LED is a minimal output pin abstraction.
type LED interface {
	High()
	Low()
}

var ErrNotImplemented = errors.New("not implemented")

// PixelFormat defines the framebuffer pixel encoding.
type PixelFormat uint8

const (
	// PixelFormatRGB565 is 16bpp: rrrrrggggggbbbbb.
	PixelFormatRGB565 PixelFormat = iota + 1
)

// Framebuffer is a simple pixel buffer plus a "present" hook.
type Framebuffer interface {
	Width() int
	Height() int
	Format() PixelFormat
	StrideBytes() int
	Buffer() []byte
	ClearRGB(r, g, b uint8)
	Present() error
}

// KeyCode is a minimal key identifier.
type KeyCode uint16

const (
	KeyUnknown KeyCode = iota
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeyEnter
	KeyEscape
	KeyBackspace
	KeyTab
	KeyDelete
	KeyHome
	KeyEnd
	KeyF1
	KeyF2
	KeyF3
)

// KeyEvent is a keyboard event.
type KeyEvent struct {
	Code  KeyCode
	Press bool
	Rune  rune
}

// Keyboard provides key events (best-effort on each platform).
type Keyboard interface {
	Events() <-chan KeyEvent
}

// Display provides access to the framebuffer (if available).
type Display interface {
	Framebuffer() Framebuffer
}

// Input provides access to input devices (if available).
type Input interface {
	Keyboard() Keyboard
}

// Flash provides raw access to non-volatile memory.
//
// It is intentionally low-level: addresses and erase blocks only.
type Flash interface {
	SizeBytes() uint32
	EraseBlockBytes() uint32
	ReadAt(p []byte, off uint32) (int, error)
	WriteAt(p []byte, off uint32) (int, error)
	Erase(off, size uint32) error
}

// Time provides a base tick stream.
//
// The tick duration is platform-defined; higher-level timers live in userland.
type Time interface {
	Ticks() <-chan uint64
}

// Network provides a low-level packet transport (optional).
type Network interface {
	Send(pkt []byte) error
	Recv(pkt []byte) (int, error)
}

// Audio provides optional sound output.
type Audio interface {
	// PWM returns a PWM-based audio output, or nil if unsupported.
	PWM() PWMAudio
}

// PWMAudio is a minimal audio output interface.
//
// The caller is responsible for timing (calling WriteSample at the desired sample rate).
type PWMAudio interface {
	Start(sampleRate uint32) error
	Stop() error
	SetVolume(vol uint8)
	WriteSample(sample int16)
}

// HAL provides the only contact point between the OS and the outside world.
type HAL interface {
	Logger() Logger
	LED() LED
	Display() Display
	Input() Input
	Flash() Flash
	Time() Time
	Network() Network
	Audio() Audio
}
