package audio

import (
	"errors"
	"fmt"
	"io"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"spark/hal"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"
	"spark/sparkos/tea"
)

const statusEveryTicks = 250

type Service struct {
	inCap  kernel.Capability
	vfsCap kernel.Capability
	pwm    hal.PWMAudio

	subscriberMu sync.Mutex
	subscriber   kernel.Capability

	state       uint32
	volume      uint32
	sampleRate  uint32
	total       uint32
	pos         uint32
	loopEnabled uint32

	metersMu sync.Mutex
	meters   [8]uint8
	metersSR uint32
	metersN  int
	metersC  [8]float64

	playMu sync.Mutex
	cancel chan struct{}
	done   chan struct{}
}

func New(inCap, vfsCap kernel.Capability, pwm hal.PWMAudio) *Service {
	s := &Service{inCap: inCap, vfsCap: vfsCap, pwm: pwm}
	atomic.StoreUint32(&s.state, uint32(proto.AudioStopped))
	atomic.StoreUint32(&s.volume, 255)
	return s
}

func (s *Service) Run(ctx *kernel.Context) {
	ch, ok := ctx.RecvChan(s.inCap)
	if !ok {
		return
	}

	done := make(chan struct{})
	defer close(done)
	go s.statusLoop(ctx, done)

	for msg := range ch {
		switch proto.Kind(msg.Kind) {
		case proto.MsgAudioSubscribe:
			s.handleSubscribe(ctx, msg)
		case proto.MsgAudioPlay:
			s.handlePlay(ctx, msg)
		case proto.MsgAudioPause:
			s.handlePause(ctx)
		case proto.MsgAudioStop:
			s.handleStop(ctx)
		case proto.MsgAudioSetVolume:
			s.handleSetVolume(ctx, msg)
		}
	}
}

func (s *Service) statusLoop(ctx *kernel.Context, done <-chan struct{}) {
	last := ctx.NowTick()
	lastSent := last
	for {
		select {
		case <-done:
			return
		default:
		}
		last = ctx.WaitTick(last)
		if last-lastSent < statusEveryTicks {
			continue
		}
		lastSent = last
		s.sendStatus(ctx)
	}
}

func (s *Service) handleSubscribe(ctx *kernel.Context, msg kernel.Message) {
	if !msg.Cap.Valid() {
		return
	}
	s.subscriberMu.Lock()
	s.subscriber = msg.Cap
	s.subscriberMu.Unlock()
	s.sendStatus(ctx)
}

func (s *Service) handlePlay(ctx *kernel.Context, msg kernel.Message) {
	loop, path, ok := proto.DecodeAudioPlayPayload(msg.Payload())
	if !ok || path == "" {
		return
	}

	s.playMu.Lock()
	s.stopLocked()
	cancel := make(chan struct{})
	done := make(chan struct{})
	s.cancel = cancel
	s.done = done
	atomic.StoreUint32(&s.pos, 0)
	atomic.StoreUint32(&s.loopEnabled, boolToUint32(loop))
	atomic.StoreUint32(&s.state, uint32(proto.AudioPlaying))
	s.playMu.Unlock()

	s.sendStatus(ctx)

	go func() {
		defer close(done)
		if err := s.play(ctx, path); err != nil {
			_ = err
		}
	}()
}

func (s *Service) handlePause(ctx *kernel.Context) {
	for {
		old := atomic.LoadUint32(&s.state)
		switch proto.AudioState(old) {
		case proto.AudioPlaying:
			if atomic.CompareAndSwapUint32(&s.state, old, uint32(proto.AudioPaused)) {
				if p, ok := s.pwm.(interface{ Pause() }); ok {
					p.Pause()
				}
				s.sendStatus(ctx)
				return
			}
		case proto.AudioPaused:
			if atomic.CompareAndSwapUint32(&s.state, old, uint32(proto.AudioPlaying)) {
				if p, ok := s.pwm.(interface{ Resume() }); ok {
					p.Resume()
				}
				s.sendStatus(ctx)
				return
			}
		default:
			return
		}
	}
}

func (s *Service) handleStop(ctx *kernel.Context) {
	s.playMu.Lock()
	s.stopLocked()
	atomic.StoreUint32(&s.state, uint32(proto.AudioStopped))
	atomic.StoreUint32(&s.pos, 0)
	s.playMu.Unlock()

	s.sendStatus(ctx)
}

func (s *Service) stopLocked() {
	if s.pwm != nil {
		_ = s.pwm.Stop()
	}
	cancel := s.cancel
	done := s.done
	s.cancel = nil
	s.done = nil
	if cancel != nil {
		close(cancel)
	}
	if done != nil {
		s.playMu.Unlock()
		<-done
		s.playMu.Lock()
	}
}

func (s *Service) handleSetVolume(ctx *kernel.Context, msg kernel.Message) {
	vol, ok := proto.DecodeAudioSetVolumePayload(msg.Payload())
	if !ok {
		return
	}
	atomic.StoreUint32(&s.volume, uint32(vol))
	if s.pwm != nil {
		s.pwm.SetVolume(vol)
	}
	s.sendStatus(ctx)
}

func (s *Service) play(ctx *kernel.Context, path string) error {
	defer func() {
		atomic.StoreUint32(&s.state, uint32(proto.AudioStopped))
		s.sendStatus(ctx)
	}()

	s.playMu.Lock()
	cancel := s.cancel
	s.playMu.Unlock()
	if cancel == nil {
		return nil
	}

	f, err := newIPCFile(ctx, s.vfsCap, path)
	if err != nil {
		return err
	}

	dec, err := tea.NewDecoder(f)
	if err != nil {
		return err
	}

	sr := uint32(dec.Header.SampleRate)
	if sr == 0 {
		return errors.New("audio: invalid sample rate")
	}
	atomic.StoreUint32(&s.sampleRate, sr)
	atomic.StoreUint32(&s.total, dec.Header.TotalSamples)
	atomic.StoreUint32(&s.pos, 0)

	if s.pwm != nil {
		if err := s.pwm.Start(sr); err != nil {
			return fmt.Errorf("audio: pwm start: %w", err)
		}
		s.pwm.SetVolume(uint8(atomic.LoadUint32(&s.volume)))
	}

	spb := int(dec.Header.SamplesPerBlock)
	if spb <= 0 || spb > 4096 {
		return fmt.Errorf("audio: invalid samples per block: %d", spb)
	}
	var block [4096]int16

	loopFromFile := (dec.Header.Flags & tea.FlagLoopEnabled) != 0
	loop := loopFromFile || atomic.LoadUint32(&s.loopEnabled) != 0

	paced := needsSamplePacing()
	var t *time.Ticker
	if paced {
		period := time.Second / time.Duration(sr)
		t = time.NewTicker(period)
		defer t.Stop()
	}

	for {
		select {
		case <-cancel:
			return nil
		default:
		}

		n, err := dec.DecodeBlock(block[:spb])
		if err != nil {
			if errors.Is(err, io.EOF) {
				if loop {
					if !paced && s.pwm != nil {
						_ = s.pwm.Stop()
						if err := s.pwm.Start(sr); err != nil {
							return fmt.Errorf("audio: pwm start: %w", err)
						}
						s.pwm.SetVolume(uint8(atomic.LoadUint32(&s.volume)))
					}
					if err := dec.SeekToBlock(0); err != nil {
						return err
					}
					atomic.StoreUint32(&s.pos, 0)
					continue
				}
				if !paced {
					s.waitDrain(cancel)
				}
				if s.pwm != nil {
					_ = s.pwm.Stop()
				}
				return nil
			}
			return err
		}

		s.updateMeters(block[:n], sr)

		for i := 0; i < n; i++ {
			if paced {
				select {
				case <-cancel:
					return nil
				case <-t.C:
				}
			} else {
				select {
				case <-cancel:
					return nil
				default:
				}
			}

			switch proto.AudioState(atomic.LoadUint32(&s.state)) {
			case proto.AudioPlaying:
				if s.pwm != nil {
					s.pwm.WriteSample(block[i])
				}
				if paced {
					atomic.AddUint32(&s.pos, 1)
				}
			case proto.AudioPaused:
				if paced {
					if s.pwm != nil {
						s.pwm.WriteSample(0)
					}
					continue
				}
				time.Sleep(10 * time.Millisecond)
			default:
				if s.pwm != nil {
					_ = s.pwm.Stop()
				}
				return nil
			}
		}
	}
}

func (s *Service) waitDrain(cancel <-chan struct{}) {
	p, ok := s.pwm.(interface{ PendingSamples() int })
	if !ok || p == nil {
		return
	}

	for {
		select {
		case <-cancel:
			return
		default:
		}
		if p.PendingSamples() == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (s *Service) sendStatus(ctx *kernel.Context) {
	s.subscriberMu.Lock()
	sub := s.subscriber
	s.subscriberMu.Unlock()
	if !sub.Valid() {
		return
	}

	state := proto.AudioState(atomic.LoadUint32(&s.state))
	vol := uint8(atomic.LoadUint32(&s.volume))
	sr := uint16(atomic.LoadUint32(&s.sampleRate))
	pos := uint32(atomic.LoadUint32(&s.pos))
	total := uint32(atomic.LoadUint32(&s.total))

	if !needsSamplePacing() {
		if p, ok := s.pwm.(interface{ PositionSamples() uint32 }); ok && p != nil {
			pos = p.PositionSamples()
			if total != 0 && pos > total {
				pos = total
			}
		}
	}

	payload := proto.AudioStatusPayload(state, vol, sr, pos, total)

	res := ctx.SendToCapResult(sub, uint16(proto.MsgAudioStatus), payload, kernel.Capability{})
	switch res {
	case kernel.SendOK:
		s.sendMeters(ctx, sub)
		return
	case kernel.SendErrQueueFull:
		return
	default:
		s.subscriberMu.Lock()
		if s.subscriber == sub {
			s.subscriber = kernel.Capability{}
		}
		s.subscriberMu.Unlock()
	}
}

func (s *Service) sendMeters(ctx *kernel.Context, sub kernel.Capability) {
	s.metersMu.Lock()
	m := s.meters
	s.metersMu.Unlock()

	payload := proto.AudioMetersPayload(m[:])
	_ = ctx.SendToCapResult(sub, uint16(proto.MsgAudioMeters), payload, kernel.Capability{})
}

func boolToUint32(v bool) uint32 {
	if v {
		return 1
	}
	return 0
}

func (s *Service) updateMeters(samples []int16, sampleRate uint32) {
	if len(samples) == 0 || sampleRate == 0 {
		return
	}

	const win = 256
	n := len(samples)
	if n > win {
		n = win
	}

	// 8-band center frequencies (Hz).
	freqs := [8]float64{60, 170, 310, 600, 1000, 3000, 6000, 12000}

	s.metersMu.Lock()
	if s.metersSR != sampleRate || s.metersN != n {
		s.metersSR = sampleRate
		s.metersN = n
		for i := range s.metersC {
			k := 0.5 + float64(n)*freqs[i]/float64(sampleRate)
			omega := 2 * math.Pi * k / float64(n)
			s.metersC[i] = 2 * math.Cos(omega)
		}
	}
	coeff := s.metersC
	prev := s.meters
	s.metersMu.Unlock()

	var next [8]uint8
	for b := 0; b < 8; b++ {
		var q0, q1, q2 float64
		c := coeff[b]
		for i := 0; i < n; i++ {
			x := float64(samples[i]) / 32768.0
			q0 = c*q1 - q2 + x
			q2 = q1
			q1 = q0
		}
		power := q1*q1 + q2*q2 - c*q1*q2
		if power < 0 {
			power = 0
		}
		amp := math.Sqrt(power) / float64(n)

		// Map amplitude to a pseudo-dB scale for more stable visuals.
		// Typical useful range is about -60dB..0dB; add a little gain.
		const eps = 1e-6
		const minDB = -60.0
		const gainDB = 12.0
		db := 20*math.Log10(amp+eps) + gainDB
		if db < minDB {
			db = minDB
		}
		if db > 0 {
			db = 0
		}
		level := int(((db - minDB) / (0 - minDB)) * 255)
		if level < 0 {
			level = 0
		}
		if level > 255 {
			level = 255
		}
		next[b] = uint8(level)
	}

	// Simple decay smoothing.
	for i := range next {
		if next[i] < prev[i] {
			d := int(prev[i]-next[i]) / 4
			next[i] = prev[i] - uint8(d)
		}
	}

	s.metersMu.Lock()
	s.meters = next
	s.metersMu.Unlock()
}
