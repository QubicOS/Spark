package hal

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// GPIOMode selects whether a pin is an input or output.
type GPIOMode uint8

const (
	GPIOModeInput GPIOMode = iota
	GPIOModeOutput
)

// GPIOPull selects the pull resistor configuration.
type GPIOPull uint8

const (
	GPIOPullNone GPIOPull = iota
	GPIOPullUp
	GPIOPullDown
)

// GPIOCaps declares what operations a pin supports.
type GPIOCaps uint8

const (
	GPIOCapInput GPIOCaps = 1 << iota
	GPIOCapOutput
	GPIOCapPullUp
	GPIOCapPullDown
)

// GPIO provides access to general-purpose IO pins.
//
// Implementations may return nil if GPIO is unsupported.
type GPIO interface {
	PinCount() int
	Pin(id int) GPIOPin
}

// GPIOPin is a single digital IO pin.
type GPIOPin interface {
	Name() string
	Caps() GPIOCaps
	Configure(mode GPIOMode, pull GPIOPull) error
	Read() (level bool, err error)
	Write(level bool) error
}

type nullGPIO struct{}

func (nullGPIO) PinCount() int      { return 0 }
func (nullGPIO) Pin(id int) GPIOPin { return nil }

type virtualGPIO struct {
	pins []GPIOPin
}

func newVirtualGPIO(pins []GPIOPin) GPIO {
	if len(pins) == 0 {
		return nullGPIO{}
	}
	return &virtualGPIO{pins: pins}
}

func (g *virtualGPIO) PinCount() int {
	if g == nil {
		return 0
	}
	return len(g.pins)
}

func (g *virtualGPIO) Pin(id int) GPIOPin {
	if g == nil || id < 0 || id >= len(g.pins) {
		return nil
	}
	return g.pins[id]
}

type virtualPin struct {
	mu    sync.Mutex
	name  string
	caps  GPIOCaps
	mode  GPIOMode
	pull  GPIOPull
	level bool
}

func newVirtualPin(name string, caps GPIOCaps) *virtualPin {
	return &virtualPin{
		name: name,
		caps: caps,
		mode: GPIOModeInput,
		pull: GPIOPullNone,
	}
}

func (p *virtualPin) Name() string   { return p.name }
func (p *virtualPin) Caps() GPIOCaps { return p.caps }

func (p *virtualPin) Configure(mode GPIOMode, pull GPIOPull) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	switch mode {
	case GPIOModeInput:
		if p.caps&GPIOCapInput == 0 {
			return fmt.Errorf("gpio: pin %s: input unsupported", p.name)
		}
	case GPIOModeOutput:
		if p.caps&GPIOCapOutput == 0 {
			return fmt.Errorf("gpio: pin %s: output unsupported", p.name)
		}
	default:
		return fmt.Errorf("gpio: pin %s: invalid mode", p.name)
	}

	switch pull {
	case GPIOPullNone:
	case GPIOPullUp:
		if p.caps&GPIOCapPullUp == 0 {
			return fmt.Errorf("gpio: pin %s: pull-up unsupported", p.name)
		}
	case GPIOPullDown:
		if p.caps&GPIOCapPullDown == 0 {
			return fmt.Errorf("gpio: pin %s: pull-down unsupported", p.name)
		}
	default:
		return fmt.Errorf("gpio: pin %s: invalid pull", p.name)
	}

	p.mode = mode
	p.pull = pull
	return nil
}

func (p *virtualPin) Read() (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.mode != GPIOModeInput && p.mode != GPIOModeOutput {
		return false, fmt.Errorf("gpio: pin %s: not configured", p.name)
	}
	return p.level, nil
}

func (p *virtualPin) Write(level bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.mode != GPIOModeOutput {
		return fmt.Errorf("gpio: pin %s: not in output mode", p.name)
	}
	p.level = level
	return nil
}

type signalPin struct {
	mu   sync.Mutex
	name string

	mode GPIOMode
	pull GPIOPull

	t0     time.Time
	now    func() time.Time
	period time.Duration
	high   time.Duration
}

func newSignalPin(name string, period, high time.Duration) GPIOPin {
	return newSignalPinWithClock(name, period, high, time.Now)
}

func newSignalPinWithClock(name string, period, high time.Duration, now func() time.Time) GPIOPin {
	if strings.TrimSpace(name) == "" {
		return nil
	}
	if now == nil {
		now = time.Now
	}
	if period <= 0 {
		period = 1 * time.Second
	}
	if high < 0 {
		high = 0
	}
	if high > period {
		high = period
	}
	return &signalPin{
		name:   name,
		mode:   GPIOModeInput,
		pull:   GPIOPullNone,
		t0:     now(),
		now:    now,
		period: period,
		high:   high,
	}
}

func (p *signalPin) Name() string   { return p.name }
func (p *signalPin) Caps() GPIOCaps { return GPIOCapInput }

func (p *signalPin) Configure(mode GPIOMode, pull GPIOPull) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if mode != GPIOModeInput {
		return fmt.Errorf("gpio: pin %s: only input supported", p.name)
	}
	if pull != GPIOPullNone {
		return fmt.Errorf("gpio: pin %s: pull unsupported", p.name)
	}
	p.mode = mode
	p.pull = pull
	return nil
}

func (p *signalPin) Read() (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.mode != GPIOModeInput {
		return false, fmt.Errorf("gpio: pin %s: not configured for input", p.name)
	}
	if p.now == nil {
		return false, fmt.Errorf("gpio: pin %s: no clock", p.name)
	}
	if p.period <= 0 {
		return false, fmt.Errorf("gpio: pin %s: invalid period", p.name)
	}

	elapsed := p.now().Sub(p.t0)
	if elapsed < 0 {
		elapsed = -elapsed
	}
	phase := elapsed % p.period
	return phase < p.high, nil
}

func (p *signalPin) Write(level bool) error {
	_ = level
	return fmt.Errorf("gpio: pin %s: output unsupported", p.name)
}

type ledPin struct {
	mu    sync.Mutex
	led   LED
	name  string
	level bool
}

func newLEDPin(name string, led LED) GPIOPin {
	if led == nil {
		return nil
	}
	return &ledPin{led: led, name: name}
}

func (p *ledPin) Name() string   { return p.name }
func (p *ledPin) Caps() GPIOCaps { return GPIOCapOutput }

func (p *ledPin) Configure(mode GPIOMode, pull GPIOPull) error {
	if mode != GPIOModeOutput {
		return fmt.Errorf("gpio: pin %s: only output supported", p.name)
	}
	if pull != GPIOPullNone {
		return fmt.Errorf("gpio: pin %s: pull unsupported", p.name)
	}
	return nil
}

func (p *ledPin) Read() (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.level, nil
}

func (p *ledPin) Write(level bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.level = level
	if level {
		p.led.High()
	} else {
		p.led.Low()
	}
	return nil
}
