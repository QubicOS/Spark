//go:build tinygo && baremetal && picocalc

package hal

import (
	"fmt"
	"machine"
	"time"
)

const (
	picoCalcKbdAddr uint16 = 0x1F
	picoCalcKbdCmd         = 0x09
)

const (
	picoCalcKeyAlt       byte = 0xA1
	picoCalcKeyBackspace byte = 0x08
	picoCalcKeyCtrl      byte = 0xA5
	picoCalcKeyDel       byte = 0xD4
	picoCalcKeyEnd       byte = 0xD5
	picoCalcKeyEsc       byte = 0xB1
	picoCalcKeyF1        byte = 0x81
	picoCalcKeyF2        byte = 0x82
	picoCalcKeyF3        byte = 0x83
	picoCalcKeyF4        byte = 0x84
	picoCalcKeyF5        byte = 0x85
	picoCalcKeyF6        byte = 0x86
	picoCalcKeyF7        byte = 0x87
	picoCalcKeyF8        byte = 0x88
	picoCalcKeyF9        byte = 0x89
	picoCalcKeyF10       byte = 0x90 // Oddly not 0x8A on PicoCalc.
	picoCalcKeyHome      byte = 0xD2
	picoCalcKeyIns       byte = 0xD1
	picoCalcKeyLeft      byte = 0xB4
	picoCalcKeyRight     byte = 0xB7
	picoCalcKeyUp        byte = 0xB5
	picoCalcKeyDown      byte = 0xB6
)

type i2cKeyboard struct {
	i2c   *machine.I2C
	write [1]byte
	read  [2]byte

	altDown  bool
	ctrlDown bool

	freq        uint32
	ignoreUntil time.Time
	errStreak   int
}

func initI2CKeyboard() (*i2cKeyboard, error) {
	write := [1]byte{picoCalcKbdCmd}

	// Prefer I2C1 (original PicoCalc wiring), but some TinyGo targets expose only I2C0.
	for _, bus := range []*machine.I2C{machine.I2C1, machine.I2C0} {
		if bus == nil {
			continue
		}
		for _, freq := range []uint32{100_000, 400_000} {
			if err := bus.Configure(machine.I2CConfig{
				SCL:       machine.GP7,
				SDA:       machine.GP6,
				Frequency: freq,
			}); err != nil {
				continue
			}

			k := &i2cKeyboard{
				i2c:         bus,
				write:       write,
				freq:        freq,
				ignoreUntil: time.Now().Add(300 * time.Millisecond),
			}

			// Probe the device to ensure the selected I2C instance works.
			// On boot the keyboard MCU can be slow to respond, so retry briefly.
			const probeTries = 50
			for i := 0; i < probeTries; i++ {
				if err := k.i2c.Tx(picoCalcKbdAddr, k.write[:], k.read[:]); err == nil {
					return k, nil
				}
				time.Sleep(10 * time.Millisecond)
			}
		}
	}

	return nil, fmt.Errorf("keyboard: I2C unavailable")
}

func (k *i2cKeyboard) readEvent() (KeyEvent, bool) {
	// Ignore any garbage/queued events while the keyboard MCU is still booting.
	if time.Now().Before(k.ignoreUntil) {
		_ = k.i2c.Tx(picoCalcKbdAddr, k.write[:], k.read[:])
		return KeyEvent{}, false
	}

	if err := k.i2c.Tx(picoCalcKbdAddr, k.write[:], k.read[:]); err != nil {
		k.errStreak++
		// Reconfigure periodically to recover from transient bus glitches.
		if k.errStreak >= 200 {
			_ = k.i2c.Configure(machine.I2CConfig{
				SCL:       machine.GP7,
				SDA:       machine.GP6,
				Frequency: k.freq,
			})
			k.errStreak = 0
			k.ignoreUntil = time.Now().Add(50 * time.Millisecond)
		}
		return KeyEvent{}, false
	}
	k.errStreak = 0
	if k.read[0] == 0 && k.read[1] == 0 {
		return KeyEvent{}, false
	}

	eventType := k.read[0]
	key := k.read[1]

	switch eventType {
	case 0x01: // key down
		return k.translate(key, true)
	case 0x02: // key held (mostly modifiers)
		switch key {
		case picoCalcKeyAlt:
			k.altDown = true
		case picoCalcKeyCtrl:
			k.ctrlDown = true
		}
		return KeyEvent{}, false
	case 0x03: // key up
		// PicoCalc firmware reports key-up mostly for modifier keys.
		// Emit release events only for special keys so termkbd can stop repeats.
		switch key {
		case picoCalcKeyAlt:
			k.altDown = false
			return KeyEvent{}, false
		case picoCalcKeyCtrl:
			k.ctrlDown = false
			return KeyEvent{}, false
		default:
			if kc := k.mapSpecial(key); kc != KeyUnknown {
				return KeyEvent{Press: false, Code: kc}, true
			}
			return KeyEvent{}, false
		}
	default:
		// key held or unknown: ignore (repeat handled in termkbd).
		return KeyEvent{}, false
	}
}

func (k *i2cKeyboard) translate(code byte, press bool) (KeyEvent, bool) {
	switch code {
	case picoCalcKeyAlt:
		k.altDown = press
		return KeyEvent{}, false
	case picoCalcKeyCtrl:
		k.ctrlDown = press
		return KeyEvent{}, false
	}

	if !press {
		return KeyEvent{Press: false, Code: k.mapSpecial(code)}, true
	}

	if kc, ok := k.specialKey(code); ok {
		return KeyEvent{Press: true, Code: kc}, true
	}

	r := rune(code)
	if r == '\r' {
		return KeyEvent{Press: true, Code: KeyEnter}, true
	}
	if r == '\n' {
		return KeyEvent{Press: true, Code: KeyEnter}, true
	}
	if r == 0 {
		return KeyEvent{}, false
	}
	return KeyEvent{Press: true, Rune: r}, true
}

func (k *i2cKeyboard) specialKey(code byte) (KeyCode, bool) {
	if kc := k.mapSpecial(code); kc != KeyUnknown {
		return kc, true
	}
	return KeyUnknown, false
}

func (k *i2cKeyboard) mapSpecial(code byte) KeyCode {
	switch code {
	case picoCalcKeyBackspace:
		return KeyBackspace
	case picoCalcKeyEsc:
		return KeyEscape
	case picoCalcKeyDel:
		return KeyDelete
	case picoCalcKeyHome:
		return KeyHome
	case picoCalcKeyEnd:
		return KeyEnd
	case picoCalcKeyLeft:
		return KeyLeft
	case picoCalcKeyRight:
		return KeyRight
	case picoCalcKeyUp:
		return KeyUp
	case picoCalcKeyDown:
		return KeyDown
	case picoCalcKeyF1:
		return KeyF1
	case picoCalcKeyF2:
		return KeyF2
	case picoCalcKeyF3:
		return KeyF3
	case picoCalcKeyF4, picoCalcKeyF5, picoCalcKeyF6, picoCalcKeyF7, picoCalcKeyF8, picoCalcKeyF9, picoCalcKeyF10:
		// Not mapped currently (termkbd provides VT100 mappings for F1..F3 only).
		return KeyUnknown
	case picoCalcKeyIns:
		// No direct VT100 mapping in termkbd; treat as Tab for now.
		return KeyTab
	default:
		return KeyUnknown
	}
}

func (k *i2cKeyboard) String() string {
	return fmt.Sprintf("alt=%v ctrl=%v", k.altDown, k.ctrlDown)
}
