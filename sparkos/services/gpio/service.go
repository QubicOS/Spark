package gpio

import (
	"fmt"

	"spark/hal"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

// Service provides GPIO access via IPC.
type Service struct {
	gpio hal.GPIO
	ep   kernel.Capability

	pinCount int
	pins     []hal.GPIOPin

	mode []proto.GPIOMode
	pull []proto.GPIOPull
}

func New(gpio hal.GPIO, ep kernel.Capability) *Service {
	s := &Service{gpio: gpio, ep: ep}
	s.initPins()
	return s
}

func (s *Service) initPins() {
	if s.gpio == nil {
		s.pinCount = 0
		return
	}
	s.pinCount = s.gpio.PinCount()
	if s.pinCount < 0 {
		s.pinCount = 0
	}

	s.pins = make([]hal.GPIOPin, s.pinCount)
	s.mode = make([]proto.GPIOMode, s.pinCount)
	s.pull = make([]proto.GPIOPull, s.pinCount)

	for i := 0; i < s.pinCount; i++ {
		s.pins[i] = s.gpio.Pin(i)
		s.mode[i] = proto.GPIOModeInput
		s.pull[i] = proto.GPIOPullNone
	}
}

func (s *Service) Run(ctx *kernel.Context) {
	ch, ok := ctx.RecvChan(s.ep)
	if !ok {
		return
	}

	for msg := range ch {
		switch proto.Kind(msg.Kind) {
		case proto.MsgGPIOList:
			s.handleList(ctx, msg)
		case proto.MsgGPIOConfig:
			s.handleConfig(ctx, msg)
		case proto.MsgGPIOWrite:
			s.handleWrite(ctx, msg)
		case proto.MsgGPIORead:
			s.handleRead(ctx, msg)
		}
	}
}

func (s *Service) handleList(ctx *kernel.Context, msg kernel.Message) {
	reply := msg.Cap
	if !reply.Valid() {
		return
	}
	requestID, ok := proto.DecodeGPIOListPayload(msg.Data[:msg.Len])
	if !ok {
		_ = s.sendErr(ctx, reply, proto.ErrBadMessage, proto.MsgGPIOList, 0, "decode list")
		return
	}

	for i := 0; i < s.pinCount; i++ {
		pin := s.pins[i]
		if pin == nil {
			continue
		}

		level, _ := pin.Read()
		payload := proto.GPIOListRespPayload(
			requestID,
			false,
			uint8(i),
			mapCaps(pin.Caps()),
			s.mode[i],
			s.pull[i],
			level,
		)
		_ = s.send(ctx, reply, proto.MsgGPIOListResp, payload)
	}

	_ = s.send(ctx, reply, proto.MsgGPIOListResp, proto.GPIOListRespPayload(requestID, true, 0, 0, 0, 0, false))
}

func (s *Service) handleConfig(ctx *kernel.Context, msg kernel.Message) {
	reply := msg.Cap
	if !reply.Valid() {
		return
	}

	requestID, pinID, mode, pull, ok := proto.DecodeGPIOConfigPayload(msg.Data[:msg.Len])
	if !ok {
		_ = s.sendErr(ctx, reply, proto.ErrBadMessage, proto.MsgGPIOConfig, 0, "decode config")
		return
	}

	if int(pinID) >= s.pinCount {
		_ = s.sendErr(ctx, reply, proto.ErrNotFound, proto.MsgGPIOConfig, requestID, "pin")
		return
	}
	pin := s.pins[int(pinID)]
	if pin == nil {
		_ = s.sendErr(ctx, reply, proto.ErrNotFound, proto.MsgGPIOConfig, requestID, "pin")
		return
	}

	if err := pin.Configure(mapMode(mode), mapPull(pull)); err != nil {
		_ = s.sendErr(ctx, reply, proto.ErrBadMessage, proto.MsgGPIOConfig, requestID, err.Error())
		return
	}

	s.mode[int(pinID)] = mode
	s.pull[int(pinID)] = pull

	level, _ := pin.Read()
	_ = s.send(ctx, reply, proto.MsgGPIOConfigResp, proto.GPIOConfigRespPayload(requestID, pinID, mode, pull, level))
}

func (s *Service) handleWrite(ctx *kernel.Context, msg kernel.Message) {
	reply := msg.Cap
	if !reply.Valid() {
		return
	}

	requestID, pinID, level, ok := proto.DecodeGPIOWritePayload(msg.Data[:msg.Len])
	if !ok {
		_ = s.sendErr(ctx, reply, proto.ErrBadMessage, proto.MsgGPIOWrite, 0, "decode write")
		return
	}
	if int(pinID) >= s.pinCount {
		_ = s.sendErr(ctx, reply, proto.ErrNotFound, proto.MsgGPIOWrite, requestID, "pin")
		return
	}
	pin := s.pins[int(pinID)]
	if pin == nil {
		_ = s.sendErr(ctx, reply, proto.ErrNotFound, proto.MsgGPIOWrite, requestID, "pin")
		return
	}

	if err := pin.Write(level); err != nil {
		_ = s.sendErr(ctx, reply, proto.ErrBadMessage, proto.MsgGPIOWrite, requestID, err.Error())
		return
	}
	_ = s.send(ctx, reply, proto.MsgGPIOWriteResp, proto.GPIOWriteRespPayload(requestID, pinID, level))
}

func (s *Service) handleRead(ctx *kernel.Context, msg kernel.Message) {
	reply := msg.Cap
	if !reply.Valid() {
		return
	}

	requestID, mask, ok := proto.DecodeGPIOReadPayload(msg.Data[:msg.Len])
	if !ok {
		_ = s.sendErr(ctx, reply, proto.ErrBadMessage, proto.MsgGPIORead, 0, "decode read")
		return
	}

	var levels uint32
	for i := 0; i < s.pinCount && i < 32; i++ {
		if mask&(1<<uint(i)) == 0 {
			continue
		}
		pin := s.pins[i]
		if pin == nil {
			continue
		}
		level, err := pin.Read()
		if err != nil {
			continue
		}
		if level {
			levels |= 1 << uint(i)
		}
	}

	_ = s.send(ctx, reply, proto.MsgGPIOReadResp, proto.GPIOReadRespPayload(requestID, mask, levels))
}

func (s *Service) send(ctx *kernel.Context, reply kernel.Capability, kind proto.Kind, payload []byte) error {
	for {
		res := ctx.SendToCapResult(reply, uint16(kind), payload, kernel.Capability{})
		switch res {
		case kernel.SendOK:
			return nil
		case kernel.SendErrQueueFull:
			ctx.BlockOnTick()
		default:
			return fmt.Errorf("gpio send: %s", res)
		}
	}
}

func (s *Service) sendErr(
	ctx *kernel.Context,
	reply kernel.Capability,
	code proto.ErrCode,
	ref proto.Kind,
	requestID uint32,
	detail string,
) error {
	d := []byte(detail)
	if requestID != 0 {
		d = proto.ErrorDetailWithRequestID(requestID, d)
	}
	return s.send(ctx, reply, proto.MsgError, proto.ErrorPayload(code, ref, d))
}

func mapCaps(c hal.GPIOCaps) proto.GPIOPinCaps {
	var out proto.GPIOPinCaps
	if c&hal.GPIOCapInput != 0 {
		out |= proto.GPIOPinCapInput
	}
	if c&hal.GPIOCapOutput != 0 {
		out |= proto.GPIOPinCapOutput
	}
	if c&hal.GPIOCapPullUp != 0 {
		out |= proto.GPIOPinCapPullUp
	}
	if c&hal.GPIOCapPullDown != 0 {
		out |= proto.GPIOPinCapPullDown
	}
	return out
}

func mapMode(m proto.GPIOMode) hal.GPIOMode {
	switch m {
	case proto.GPIOModeOutput:
		return hal.GPIOModeOutput
	default:
		return hal.GPIOModeInput
	}
}

func mapPull(p proto.GPIOPull) hal.GPIOPull {
	switch p {
	case proto.GPIOPullUp:
		return hal.GPIOPullUp
	case proto.GPIOPullDown:
		return hal.GPIOPullDown
	default:
		return hal.GPIOPullNone
	}
}
