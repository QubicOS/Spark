package serial

import (
	"fmt"
	"sync"

	"spark/hal"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

// Service routes UART bytes between clients and the HAL serial interface.
type Service struct {
	serial hal.Serial
	ep     kernel.Capability

	mu    sync.Mutex
	rxCap kernel.Capability
}

// New creates a serial service.
func New(serial hal.Serial, ep kernel.Capability) *Service {
	return &Service{serial: serial, ep: ep}
}

// Run handles serial requests and streams incoming data to subscribers.
func (s *Service) Run(ctx *kernel.Context) {
	ch, ok := ctx.RecvChan(s.ep)
	if !ok {
		return
	}
	if s.serial != nil {
		go s.readLoop(ctx)
	}

	for msg := range ch {
		switch proto.Kind(msg.Kind) {
		case proto.MsgSerialSubscribe:
			s.setRxCap(msg.Cap)
		case proto.MsgSerialWrite:
			if s.serial == nil || len(msg.Payload()) == 0 {
				continue
			}
			_, _ = s.serial.Write(msg.Payload())
		}
	}
}

func (s *Service) setRxCap(cap kernel.Capability) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rxCap = cap
}

func (s *Service) readLoop(ctx *kernel.Context) {
	buf := make([]byte, kernel.MaxMessageBytes)
	for {
		n, err := s.serial.Read(buf)
		if n > 0 {
			if err := s.sendData(ctx, buf[:n]); err != nil {
				_ = err
			}
		}
		if err != nil {
			ctx.BlockOnTick()
		}
	}
}

func (s *Service) sendData(ctx *kernel.Context, payload []byte) error {
	s.mu.Lock()
	cap := s.rxCap
	s.mu.Unlock()
	if !cap.Valid() {
		return nil
	}
	if len(payload) == 0 {
		return nil
	}
	for len(payload) > 0 {
		chunk := payload
		if len(chunk) > kernel.MaxMessageBytes {
			chunk = chunk[:kernel.MaxMessageBytes]
		}
		res := ctx.SendToCapRetry(cap, uint16(proto.MsgSerialData), chunk, kernel.Capability{}, 100)
		if res != kernel.SendOK {
			return fmt.Errorf("serial send data: %s", res)
		}
		payload = payload[len(chunk):]
	}
	return nil
}
