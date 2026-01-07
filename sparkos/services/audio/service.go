package audio

import (
	"errors"
	"fmt"
	"io"
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

	go s.statusLoop(ctx)

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

func (s *Service) statusLoop(ctx *kernel.Context) {
	last := ctx.NowTick()
	lastSent := last
	for {
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
	loop, path, ok := proto.DecodeAudioPlayPayload(msg.Data[:msg.Len])
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
	vol, ok := proto.DecodeAudioSetVolumePayload(msg.Data[:msg.Len])
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
				atomic.AddUint32(&s.pos, 1)
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

	payload := proto.AudioStatusPayload(
		proto.AudioState(atomic.LoadUint32(&s.state)),
		uint8(atomic.LoadUint32(&s.volume)),
		uint16(atomic.LoadUint32(&s.sampleRate)),
		uint32(atomic.LoadUint32(&s.pos)),
		uint32(atomic.LoadUint32(&s.total)),
	)

	res := ctx.SendToCapResult(sub, uint16(proto.MsgAudioStatus), payload, kernel.Capability{})
	switch res {
	case kernel.SendOK:
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

func boolToUint32(v bool) uint32 {
	if v {
		return 1
	}
	return 0
}
