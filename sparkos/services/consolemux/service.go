package consolemux

import (
	"fmt"

	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

const interruptByte = 0x07 // Ctrl+G (toggle focus between shell and app).

type Service struct {
	inCap  kernel.Capability
	ctlCap kernel.Capability

	shellCap     kernel.Capability
	rtdemoCap    kernel.Capability
	rtvoxelCap   kernel.Capability
	imgviewCap   kernel.Capability
	viCap        kernel.Capability
	mcCap        kernel.Capability
	hexCap       kernel.Capability
	vectorCap    kernel.Capability
	snakeCap     kernel.Capability
	tetrisCap    kernel.Capability
	calendarCap  kernel.Capability
	todoCap      kernel.Capability
	archiveCap   kernel.Capability
	teaCap       kernel.Capability
	basicCap     kernel.Capability
	rfCap        kernel.Capability
	gpioscopeCap kernel.Capability
	fbtestCap    kernel.Capability
	serialCap    kernel.Capability
	usersCap     kernel.Capability
	termCap      kernel.Capability

	activeApp proto.AppID
	appActive bool
}

func New(inCap, ctlCap, shellCap, rtdemoCap, rtvoxelCap, imgviewCap, viCap, mcCap, hexCap, vectorCap, snakeCap, tetrisCap, calendarCap, todoCap, archiveCap, teaCap, basicCap, rfCap, gpioscopeCap, fbtestCap, serialCap, usersCap, termCap kernel.Capability) *Service {
	return &Service{
		inCap:        inCap,
		ctlCap:       ctlCap,
		shellCap:     shellCap,
		rtdemoCap:    rtdemoCap,
		rtvoxelCap:   rtvoxelCap,
		imgviewCap:   imgviewCap,
		viCap:        viCap,
		mcCap:        mcCap,
		hexCap:       hexCap,
		vectorCap:    vectorCap,
		snakeCap:     snakeCap,
		tetrisCap:    tetrisCap,
		calendarCap:  calendarCap,
		todoCap:      todoCap,
		archiveCap:   archiveCap,
		teaCap:       teaCap,
		basicCap:     basicCap,
		rfCap:        rfCap,
		gpioscopeCap: gpioscopeCap,
		fbtestCap:    fbtestCap,
		serialCap:    serialCap,
		usersCap:     usersCap,
		termCap:      termCap,
		activeApp:    proto.AppRTDemo,
	}
}

func (s *Service) Run(ctx *kernel.Context) {
	ch, ok := ctx.RecvChan(s.inCap)
	if !ok {
		return
	}

	for msg := range ch {
		switch proto.Kind(msg.Kind) {
		case proto.MsgTermInput:
			s.handleInput(ctx, msg.Payload())
		case proto.MsgAppControl:
			active, ok := proto.DecodeAppControlPayload(msg.Payload())
			if !ok {
				continue
			}
			s.setActive(ctx, active)
		case proto.MsgAppSelect:
			appID, arg, ok := proto.DecodeAppSelectPayload(msg.Payload())
			if !ok {
				continue
			}
			s.handleAppSelect(ctx, appID, arg)
		}
	}
}

func (s *Service) handleInput(ctx *kernel.Context, b []byte) {
	start := 0
	for i := 0; i < len(b); i++ {
		switch b[i] {
		case interruptByte:
			s.flushInput(ctx, b[start:i])
			start = i + 1
			s.setActive(ctx, !s.appActive)
		}
	}
	s.flushInput(ctx, b[start:])
}

func (s *Service) flushInput(ctx *kernel.Context, b []byte) {
	if len(b) == 0 {
		return
	}
	dst := s.shellCap
	if s.appActive {
		dst = s.selectedAppCap()
		if !dst.Valid() {
			s.setActive(ctx, false)
			dst = s.shellCap
		} else {
			if err := sendWithRetry(ctx, dst, proto.MsgTermInput, b, kernel.Capability{}); err == nil {
				return
			}
			s.setActive(ctx, false)
			dst = s.shellCap
		}
	}
	_ = sendWithRetry(ctx, dst, proto.MsgTermInput, b, kernel.Capability{})
}

func (s *Service) setActive(ctx *kernel.Context, active bool) {
	if active == s.appActive {
		return
	}
	if active && !s.selectedAppCap().Valid() {
		return
	}
	s.appActive = active

	appCap := s.selectedAppCap()
	var xfer kernel.Capability
	if s.appActive && s.ctlCap.Valid() {
		xfer = s.ctlCap
	}
	_ = sendWithRetry(ctx, appCap, proto.MsgAppControl, proto.AppControlPayload(active), xfer)
	_ = sendWithRetry(ctx, s.shellCap, proto.MsgAppControl, proto.AppControlPayload(!active), kernel.Capability{})
}

func (s *Service) handleAppSelect(ctx *kernel.Context, id proto.AppID, arg string) {
	appCap := s.appCapByID(id)
	if !appCap.Valid() {
		return
	}
	s.activeApp = id
	_ = sendWithRetry(ctx, appCap, proto.MsgAppSelect, proto.AppSelectPayload(id, arg), kernel.Capability{})
}

func (s *Service) selectedAppCap() kernel.Capability {
	return s.appCapByID(s.activeApp)
}

func (s *Service) appCapByID(id proto.AppID) kernel.Capability {
	switch id {
	case proto.AppRTDemo:
		return s.rtdemoCap
	case proto.AppRTVoxel:
		return s.rtvoxelCap
	case proto.AppImgView:
		return s.imgviewCap
	case proto.AppVi:
		return s.viCap
	case proto.AppMC:
		return s.mcCap
	case proto.AppHex:
		return s.hexCap
	case proto.AppVector:
		return s.vectorCap
	case proto.AppSnake:
		return s.snakeCap
	case proto.AppTetris:
		return s.tetrisCap
	case proto.AppCalendar:
		return s.calendarCap
	case proto.AppTodo:
		return s.todoCap
	case proto.AppArchive:
		return s.archiveCap
	case proto.AppTEA:
		return s.teaCap
	case proto.AppBasic:
		return s.basicCap
	case proto.AppRFAnalyzer:
		return s.rfCap
	case proto.AppGPIOScope:
		return s.gpioscopeCap
	case proto.AppFBTest:
		return s.fbtestCap
	case proto.AppSerialTerm:
		return s.serialCap
	case proto.AppUsers:
		return s.usersCap
	default:
		return kernel.Capability{}
	}
}

const sendRetryLimit = 500

func sendWithRetry(ctx *kernel.Context, toCap kernel.Capability, kind proto.Kind, payload []byte, xfer kernel.Capability) error {
	if !toCap.Valid() {
		return nil
	}
	if len(payload) == 0 {
		retries := 0
		for {
			res := ctx.SendToCapResult(toCap, uint16(kind), nil, xfer)
			switch res {
			case kernel.SendOK:
				return nil
			case kernel.SendErrQueueFull:
				retries++
				if retries >= sendRetryLimit {
					return fmt.Errorf("consolemux send %s: queue full", kind)
				}
				ctx.BlockOnTick()
			default:
				return fmt.Errorf("consolemux send %s: %s", kind, res)
			}
		}
	}

	for len(payload) > 0 {
		chunk := payload
		if len(chunk) > kernel.MaxMessageBytes {
			chunk = chunk[:kernel.MaxMessageBytes]
		}

		retries := 0
		for {
			res := ctx.SendToCapResult(toCap, uint16(kind), chunk, xfer)
			switch res {
			case kernel.SendOK:
				payload = payload[len(chunk):]
				goto nextChunk
			case kernel.SendErrQueueFull:
				retries++
				if retries >= sendRetryLimit {
					return fmt.Errorf("consolemux send %s: queue full", kind)
				}
				ctx.BlockOnTick()
			default:
				return fmt.Errorf("consolemux send %s: %s", kind, res)
			}
		}
	nextChunk:
	}
	return nil
}
