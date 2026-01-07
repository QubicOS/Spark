//go:build tinygo && baremetal

package hal

import (
	"machine"
)

type tinyGoAudio struct {
	pwm *pwmAudioOut
}

func newTinyGoAudio() Audio {
	return &tinyGoAudio{pwm: newPWMAudioOut(machine.GP2)}
}

func (a *tinyGoAudio) PWM() PWMAudio { return a.pwm }

type pwmDevice interface {
	Configure(config machine.PWMConfig) error
	Channel(pin machine.Pin) (uint8, error)
	SetTop(top uint32)
	Top() uint32
	Set(channel uint8, value uint32)
	Enable(enable bool)
}

type pwmAudioOut struct {
	pin machine.Pin
	pwm pwmDevice
	ch  uint8
	top uint32

	volume  uint8
	started bool
}

func newPWMAudioOut(pin machine.Pin) *pwmAudioOut {
	pwm := pwmForPin(pin)
	if pwm == nil {
		return nil
	}
	return &pwmAudioOut{pin: pin, pwm: pwm, volume: 255}
}

func pwmForPin(pin machine.Pin) pwmDevice {
	slice, err := machine.PWMPeripheral(pin)
	if err != nil {
		return nil
	}
	switch slice {
	case 0:
		return machine.PWM0
	case 1:
		return machine.PWM1
	case 2:
		return machine.PWM2
	case 3:
		return machine.PWM3
	case 4:
		return machine.PWM4
	case 5:
		return machine.PWM5
	case 6:
		return machine.PWM6
	case 7:
		return machine.PWM7
	default:
		return nil
	}
}

func (a *pwmAudioOut) Start(sampleRate uint32) error {
	if a == nil || a.pwm == nil {
		return ErrNotImplemented
	}
	if sampleRate == 0 {
		return ErrNotImplemented
	}
	// Use a fixed PWM carrier (~62.5kHz) and update duty at the audio sample rate.
	const pwmCarrierHz = 62500
	if err := a.pwm.Configure(machine.PWMConfig{Period: 1e9 / pwmCarrierHz}); err != nil {
		return err
	}
	ch, err := a.pwm.Channel(a.pin)
	if err != nil {
		return err
	}
	a.ch = ch
	a.pwm.SetTop(0xFFFF)
	a.top = a.pwm.Top()
	a.pwm.Set(a.ch, a.top/2)
	a.pwm.Enable(true)
	a.started = true
	return nil
}

func (a *pwmAudioOut) Stop() error {
	if a == nil || a.pwm == nil {
		return nil
	}
	a.pwm.Set(a.ch, a.top/2)
	a.pwm.Enable(false)
	a.started = false
	return nil
}

func (a *pwmAudioOut) SetVolume(vol uint8) {
	if a == nil {
		return
	}
	a.volume = vol
}

func (a *pwmAudioOut) WriteSample(sample int16) {
	if a == nil || a.pwm == nil || !a.started {
		return
	}
	vol := uint32(a.volume)
	s := int32(sample)
	s = (s * int32(vol)) / 255
	u := uint32(int32(s) + 32768)
	duty := (u * a.top) / 65535
	a.pwm.Set(a.ch, duty)
}
