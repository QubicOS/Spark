// package tinyterm is for TinyGo developers who want to use
// a terminal style user interface for any display that supports the
// Displayer interface. This includes several different displays in the
// TinyGo Drivers repo.
package tinyterm

import (
	"bytes"
	"fmt"
	"image/color"
	"strconv"
	"strings"
	"unicode/utf8"

	"tinygo.org/x/drivers"
	"tinygo.org/x/tinyfont"
)

// NewTerminal returns a new Terminal. The Terminal will need to
// have Configure called on it to be used.
func NewTerminal(display Displayer) *Terminal {
	return &Terminal{
		display: display,
	}
}

// Displayer is a wrapper around the TinyGo drivers repo's Displayer interface.
type Displayer interface {
	drivers.Displayer
	FillRectangle(x, y, width, height int16, c color.RGBA) error
	SetScroll(line int16)
	SetRotation(rotation drivers.Rotation) error
}

// Terminal is a terminal interface that can be used on any display
// that supports the Displayer interface.
type Terminal struct {
	display Displayer
	width   int16
	height  int16
	scroll  int16

	rows int16 // number of rows in the text buffer
	cols int16 // number of columns in the text buffer
	next int16 // index in the buffer at which next char will be put

	state   state
	command byte
	params  *bytes.Buffer
	attrs   sgrAttrs

	font       tinyfont.Fonter
	fontWidth  int16
	fontHeight int16
	fontOffset int16

	useSoftwareScroll bool
}

type clipDisplayer struct {
	base Displayer
	x0   int16
	y0   int16
	x1   int16
	y1   int16
}

func (d clipDisplayer) Size() (x, y int16) { return d.base.Size() }

func (d clipDisplayer) SetPixel(x, y int16, c color.RGBA) {
	if x < d.x0 || x >= d.x1 || y < d.y0 || y >= d.y1 {
		return
	}
	d.base.SetPixel(x, y, c)
}

func (d clipDisplayer) Display() error { return d.base.Display() }

func (d clipDisplayer) FillRectangle(x, y, width, height int16, c color.RGBA) error {
	return d.base.FillRectangle(x, y, width, height, c)
}

func (d clipDisplayer) SetScroll(line int16) { d.base.SetScroll(line) }

func (d clipDisplayer) SetRotation(rotation drivers.Rotation) error {
	return d.base.SetRotation(rotation)
}

// Config contains the configuration for a Terminal.
type Config struct {
	// the font to be used for the terminal
	Font tinyfont.Fonter

	// font height for the terminal
	FontHeight int16

	// font offset for the terminal
	FontOffset int16

	// UseSoftwareScroll when true will blank the display an start again at
	// the top when running out of space, instead of using whatever hardware
	// scrolling is available on the display.
	UseSoftwareScroll bool
}

// Configure needs to be called for a new Terminal before it can be used.
func (t *Terminal) Configure(config *Config) {
	t.state = stateInput
	t.params = bytes.NewBuffer(make([]byte, 32))

	t.attrs.reset()

	_, charWidth := tinyfont.LineWidth(config.Font, "0")

	t.font = config.Font
	t.fontWidth = int16(charWidth)
	t.fontHeight = config.FontHeight
	t.fontOffset = config.FontOffset

	t.width, t.height = t.display.Size()
	t.rows = t.height / t.fontHeight
	t.cols = t.width / t.fontWidth

	t.useSoftwareScroll = config.UseSoftwareScroll
	t.scroll = 0
	t.next = 0
	if !t.useSoftwareScroll {
		t.display.SetScroll(0)
	}
	_ = t.display.FillRectangle(0, t.scroll, t.width, t.fontHeight, t.attrs.bgcol)
}

// Write some data to the terminal.
func (t *Terminal) Write(buf []byte) (int, error) {
	for i := 0; i < len(buf); {
		b := buf[i]
		if t.state != stateInput {
			t.putchar(b)
			i++
			continue
		}

		switch b {
		case 0x1b, '\r', '\n':
			t.putchar(b)
			i++
			continue
		default:
		}

		r, sz := utf8.DecodeRune(buf[i:])
		if r == utf8.RuneError && sz == 1 {
			t.drawrune(rune(b))
			i++
			continue
		}
		t.drawrune(r)
		i += sz
	}
	return len(buf), nil
}

// Write a single byte to the terminal.
func (t *Terminal) WriteByte(b byte) error {
	t.putchar(b)
	return nil
}

// Printf wraps the fmt package function of the same name, and outputs the
// result to the terminal.
func (t *Terminal) Printf(format string, args ...interface{}) (n int, err error) {
	return fmt.Fprintf(t, format, args...)
}

// Println wraps the fmt package function of the same name, and outputs the
// result to the terminal.
func (t *Terminal) Println(args ...interface{}) (n int, err error) {
	return fmt.Fprintln(t, args...)
}

// Display the terminal on the display. Must be called after writing to the
// terminal to see the changes.
func (t *Terminal) Display() {
	t.display.Display()
}

type state uint8

const (
	stateInput state = iota
	stateEscape
	stateCSI
)

func (t *Terminal) putchar(b byte) {
	switch t.state {
	case stateInput:
		switch b {
		case 0x1b:
			t.state = stateEscape
			return
		case '\r':
			t.cr()
			return
		case '\n':
			t.lf()
			return
		default:
			t.drawrune(rune(b))
			return
		}

	case stateEscape:
		switch b {
		case 'N':
			// SS2: Single Shift Two
			t.state = stateInput
		case 'O':
			// SS3: Single Shift Three
			t.state = stateInput
		case 'P':
			// DCS: Device Control String
			t.command = b
		case '[':
			// CSI: Control Sequence Introducer
			t.params.Reset()
			t.state = stateCSI
		case '\\':
			// ST: String Terminator
			t.state = stateInput
		case ']':
			// OSC: Operating System Command
			t.command = b
		case 'X':
			// SOS: Start of String
			t.command = b
		case '^':
			// PM: Privacy Message
			t.command = b
		case '_':
			// APC: Application Program Command
			t.command = b
		case 'c':
			// RIS: Reset to Initial State
			// TODO: need to implement
			t.state = stateInput
		}
	case stateCSI:
		switch {
		case b >= 0x20 && b <= 0x2F:
			// intermediate bytes
			t.params.WriteByte(b)
		case b >= 0x30 && b <= 0x3F:
			// parameter bytes
			t.params.WriteByte(b)
		default:
			// final bytes
			switch b {
			case 'A':
				// CUU: Cursor Up
			case 'B':
				// CUD: Cursor Down
			case 'C':
				// CUF: Cursor Forward
				t.cursorForward()
			case 'D':
				// CUB: Cursor Back
				t.cursorBack()
			case 'E':
				// CNL: Cursor Next Line
			case 'F':
				// CPL: Cursor Previous Line
			case 'G':
				// CHA: Cursor Horizontal Absolute
				t.cursorHorizontalAbsolute()
			case 'H':
				// CUP: Cursor Position
			case 'J':
				// ED: Erase in Display
			case 'K':
				// EL: Erase in Line
				t.eraseInLine()
			case 'S':
				// SU: Scroll Up
			case 'T':
				// SD: Scroll Down
			case 'f':
				// HVP: Horizontal Vertical Position
			case 'm':
				// SGR: Select Graphic Rendition
				t.selectGraphicRendition()
			case 'i':
				// AUX Port
			case 'n':
				// DSR: Device Status Report
			default:
				// undefined behavior; just reset the sequence to input mode
			}
			t.state = stateInput
		}
	}
}

type scrollUpper interface {
	ScrollUp(pixels int16, bg color.RGBA) error
}

func (t *Terminal) selectGraphicRendition() {
	params := strings.Split(t.params.String(), ";")
	attr, err := strconv.Atoi(params[0])
	if err != nil {
		println("error converting SGR param ID: " + err.Error())
	}
	switch attr {
	case SGRReset:
		t.attrs.reset()
	case SGRBold:
		t.attrs.attrs |= byte(attr)
	case SGRFgBlack:
		fallthrough
	case SGRFgRed:
		fallthrough
	case SGRFgGreen:
		fallthrough
	case SGRFgYellow:
		fallthrough
	case SGRFgBlue:
		fallthrough
	case SGRFgMagenta:
		fallthrough
	case SGRFgCyan:
		fallthrough
	case SGRFgWhite:
		t.attrs.setFG(Color(attr % 10))
	case SGRSetFgColor:
		c, err := strconv.Atoi(params[2])
		if err != nil {
			println("error converting color: " + err.Error())
		}
		t.attrs.setFG(Color(c))
	case SGRDefaultFgColor:
		t.attrs.setFG(ColorWhite)
	case SGRBgBlack:
		fallthrough
	case SGRBgRed:
		fallthrough
	case SGRBgGreen:
		fallthrough
	case SGRBgYellow:
		fallthrough
	case SGRBgBlue:
		fallthrough
	case SGRBgMagenta:
		fallthrough
	case SGRBgCyan:
		fallthrough
	case SGRBgWhite:
		t.attrs.setBG(Color(attr % 10))
	case SGRSetBgColor:
		c, err := strconv.Atoi(params[2])
		if err != nil {
			println("error converting color: " + err.Error())
		}
		t.attrs.setBG(Color(c))
	case SGRDefaultBgColor:
		t.attrs.setBG(ColorBlack)
	}
}

func (t *Terminal) drawrune(r rune) {
	if t.next == t.cols {
		t.lf()
	}
	x := t.next * t.fontWidth
	y := t.scroll + t.fontOffset
	// Clear slightly wider than a single cell: many fonts have negative XOffset and
	// can paint a few pixels into the previous cell, which would otherwise "stick"
	// after backspace/redraw.
	const padX = int16(2)
	t.display.FillRectangle(x-padX, t.scroll, t.fontWidth+padX*2, t.fontHeight, t.attrs.bgcol)
	cell := clipDisplayer{
		base: t.display,
		x0:   x - padX,
		y0:   t.scroll,
		x1:   x + t.fontWidth + padX,
		y1:   t.scroll + t.fontHeight,
	}
	tinyfont.DrawChar(cell, t.font, x, y, r, t.attrs.fgcol)
	t.next += 1
}

func (t *Terminal) cursorBack() {
	if t.next > 0 {
		t.next -= 1
	}
}

func (t *Terminal) cursorForward() {
	if t.next < t.cols-1 {
		t.next += 1
	}
}

func (t *Terminal) cr() {
}

func (t *Terminal) lf() {
	t.next = 0
	if t.useSoftwareScroll {
		usableHeight := t.rows * t.fontHeight
		if usableHeight <= 0 {
			usableHeight = t.height
		}

		if t.scroll+t.fontHeight >= usableHeight {
			scroller, ok := t.display.(scrollUpper)
			if ok {
				_ = scroller.ScrollUp(t.fontHeight, t.attrs.bgcol)
			} else {
				_ = t.display.FillRectangle(0, 0, t.width, t.height, t.attrs.bgcol)
			}
			t.scroll = usableHeight - t.fontHeight
		} else {
			t.scroll += t.fontHeight
		}
	} else {
		t.scroll = (t.scroll + t.fontHeight) % (t.rows * t.fontHeight)
		t.display.SetScroll((t.scroll + t.fontHeight) % t.height)
	}
	t.display.FillRectangle(0, t.scroll, t.width, t.fontHeight, t.attrs.bgcol)
}

func (t *Terminal) eraseInLine() {
	mode := 0
	if p := strings.TrimSpace(t.params.String()); p != "" {
		params := strings.Split(p, ";")
		n, err := strconv.Atoi(params[0])
		if err == nil {
			mode = n
		}
	}

	x := t.next * t.fontWidth
	switch mode {
	case 0:
		_ = t.display.FillRectangle(x, t.scroll, t.width-x, t.fontHeight, t.attrs.bgcol)
	case 1:
		_ = t.display.FillRectangle(0, t.scroll, x+t.fontWidth, t.fontHeight, t.attrs.bgcol)
	case 2:
		_ = t.display.FillRectangle(0, t.scroll, t.width, t.fontHeight, t.attrs.bgcol)
	default:
		_ = t.display.FillRectangle(x, t.scroll, t.width-x, t.fontHeight, t.attrs.bgcol)
	}
}

func (t *Terminal) cursorHorizontalAbsolute() {
	col := 1
	if p := strings.TrimSpace(t.params.String()); p != "" {
		params := strings.Split(p, ";")
		n, err := strconv.Atoi(params[0])
		if err == nil && n > 0 {
			col = n
		}
	}

	n := int16(col - 1)
	if n < 0 {
		n = 0
	}
	if n >= t.cols {
		n = t.cols - 1
	}
	t.next = n
}
