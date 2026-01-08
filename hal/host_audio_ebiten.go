//go:build !tinygo && cgo

package hal

import (
	"errors"
	"io"
	"sync"
	"time"

	"github.com/hajimehoshi/ebiten/v2/audio"
)

// hostAudio exposes audio output on desktop via Ebiten's audio package.
type hostAudio struct {
	pwm *hostPWMAudio
}

func newHostAudio() hostAudio {
	return hostAudio{pwm: &hostPWMAudio{}}
}

func (a hostAudio) PWM() PWMAudio { return a.pwm }

type hostPWMAudio struct {
	mu   sync.Mutex
	cond *sync.Cond

	ctx        *audio.Context
	player     *audio.Player
	sampleRate uint32

	buf []int16
	r   int
	w   int
	n   int

	closed bool
	vol    uint8
}

func (a *hostPWMAudio) Start(sampleRate uint32) error {
	if sampleRate == 0 {
		return errors.New("host audio: invalid sample rate")
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if a.cond == nil {
		a.cond = sync.NewCond(&a.mu)
	}

	if a.ctx == nil {
		a.ctx = audio.NewContext(int(sampleRate))
	} else if a.ctx.SampleRate() != int(sampleRate) {
		return errors.New("host audio: ebiten audio context sample rate is fixed")
	}
	a.sampleRate = sampleRate

	if a.player != nil {
		_ = a.player.Close()
		a.player = nil
	}

	ring := int(sampleRate / 10) // ~100ms buffer.
	if ring < 2048 {
		ring = 2048
	}
	if ring > 16384 {
		ring = 16384
	}
	a.buf = make([]int16, ring)
	a.r, a.w, a.n = 0, 0, 0
	a.closed = false

	p, err := a.ctx.NewPlayer(&hostAudioReader{a: a})
	if err != nil {
		return err
	}
	p.SetBufferSize(100 * time.Millisecond)
	p.SetVolume(float64(a.vol) / 255.0)
	p.Play()
	a.player = p
	return nil
}

func (a *hostPWMAudio) Stop() error {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return nil
	}
	a.closed = true
	a.n = 0
	a.r = 0
	a.w = 0
	if a.cond != nil {
		a.cond.Broadcast()
	}
	p := a.player
	a.player = nil
	a.mu.Unlock()

	if p != nil {
		return p.Close()
	}
	return nil
}

func (a *hostPWMAudio) SetVolume(vol uint8) {
	a.mu.Lock()
	a.vol = vol
	p := a.player
	a.mu.Unlock()

	if p != nil {
		p.SetVolume(float64(vol) / 255.0)
	}
}

func (a *hostPWMAudio) WriteSample(sample int16) {
	a.mu.Lock()
	for !a.closed && a.n == len(a.buf) {
		a.cond.Wait()
	}
	if a.closed || len(a.buf) == 0 {
		a.mu.Unlock()
		return
	}
	a.buf[a.w] = sample
	a.w++
	if a.w >= len(a.buf) {
		a.w = 0
	}
	a.n++
	a.cond.Signal()
	a.mu.Unlock()
}

func (a *hostPWMAudio) PendingSamples() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.n
}

func (a *hostPWMAudio) PositionSamples() uint32 {
	a.mu.Lock()
	p := a.player
	sr := a.sampleRate
	a.mu.Unlock()

	if p == nil || sr == 0 {
		return 0
	}

	sec := p.Position().Seconds()
	if sec <= 0 {
		return 0
	}
	return uint32(sec * float64(sr))
}

func (a *hostPWMAudio) Pause() {
	a.mu.Lock()
	p := a.player
	a.mu.Unlock()
	if p != nil {
		p.Pause()
	}
}

func (a *hostPWMAudio) Resume() {
	a.mu.Lock()
	p := a.player
	a.mu.Unlock()
	if p != nil {
		p.Play()
	}
}

type hostAudioReader struct {
	a *hostPWMAudio
}

func (r *hostAudioReader) Read(p []byte) (int, error) {
	a := r.a
	// Ebiten audio expects 16-bit little-endian stereo.
	for i := 0; i+3 < len(p); i += 4 {
		var s int16

		a.mu.Lock()
		for !a.closed && a.n == 0 {
			a.cond.Wait()
		}
		if a.closed {
			a.mu.Unlock()
			return i, io.EOF
		}
		if a.n > 0 {
			s = a.buf[a.r]
			a.r++
			if a.r >= len(a.buf) {
				a.r = 0
			}
			a.n--
			a.cond.Signal()
		}
		a.mu.Unlock()

		p[i+0] = byte(s)
		p[i+1] = byte(s >> 8)
		p[i+2] = byte(s)
		p[i+3] = byte(s >> 8)
	}
	return len(p), nil
}
