package appmgr

import (
	"spark/hal"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"
	mctask "spark/sparkos/tasks/mc"
	vectortask "spark/sparkos/tasks/vector"
	vitask "spark/sparkos/tasks/vi"
)

type Service struct {
	disp   hal.Display
	vfsCap kernel.Capability

	viProxyCap     kernel.Capability
	mcProxyCap     kernel.Capability
	vectorProxyCap kernel.Capability

	viCap     kernel.Capability
	mcCap     kernel.Capability
	vectorCap kernel.Capability

	viEP     kernel.Capability
	mcEP     kernel.Capability
	vectorEP kernel.Capability

	viRunning     bool
	mcRunning     bool
	vectorRunning bool
}

func New(disp hal.Display, vfsCap, viProxyCap, mcProxyCap, vectorProxyCap, viCap, mcCap, vectorCap, viEP, mcEP, vectorEP kernel.Capability) *Service {
	return &Service{
		disp:           disp,
		vfsCap:         vfsCap,
		viProxyCap:     viProxyCap,
		mcProxyCap:     mcProxyCap,
		vectorProxyCap: vectorProxyCap,
		viCap:          viCap,
		mcCap:          mcCap,
		vectorCap:      vectorCap,
		viEP:           viEP,
		mcEP:           mcEP,
		vectorEP:       vectorEP,
	}
}

func (s *Service) Run(ctx *kernel.Context) {
	go s.runProxy(ctx, s.viProxyCap, proto.AppVi)
	go s.runProxy(ctx, s.mcProxyCap, proto.AppMC)
	go s.runProxy(ctx, s.vectorProxyCap, proto.AppVector)
	select {}
}

func (s *Service) runProxy(ctx *kernel.Context, proxyCap kernel.Capability, appID proto.AppID) {
	ch, ok := ctx.RecvChan(proxyCap)
	if !ok {
		return
	}
	for msg := range ch {
		switch proto.Kind(msg.Kind) {
		case proto.MsgAppSelect:
			s.ensureRunning(ctx, appID)
			_ = ctx.SendToCapResult(s.appCapByID(appID), msg.Kind, msg.Data[:msg.Len], kernel.Capability{})

		case proto.MsgAppControl:
			active, ok := proto.DecodeAppControlPayload(msg.Data[:msg.Len])
			if !ok {
				continue
			}
			if active {
				s.ensureRunning(ctx, appID)
				_ = ctx.SendToCapResult(s.appCapByID(appID), msg.Kind, msg.Data[:msg.Len], msg.Cap)
				continue
			}

			_ = ctx.SendToCapResult(s.appCapByID(appID), msg.Kind, msg.Data[:msg.Len], kernel.Capability{})
			s.stop(ctx, appID)

		case proto.MsgTermInput:
			s.ensureRunning(ctx, appID)
			_ = ctx.SendToCapResult(s.appCapByID(appID), msg.Kind, msg.Data[:msg.Len], kernel.Capability{})
		}
	}
}

func (s *Service) ensureRunning(ctx *kernel.Context, appID proto.AppID) {
	switch appID {
	case proto.AppVi:
		if s.viRunning {
			return
		}
		ctx.AddTask(vitask.New(s.disp, s.viEP, s.vfsCap))
		s.viRunning = true

	case proto.AppMC:
		if s.mcRunning {
			return
		}
		ctx.AddTask(mctask.New(s.disp, s.mcEP, s.vfsCap))
		s.mcRunning = true

	case proto.AppVector:
		if s.vectorRunning {
			return
		}
		ctx.AddTask(vectortask.New(s.disp, s.vectorEP))
		s.vectorRunning = true
	}
}

func (s *Service) stop(ctx *kernel.Context, appID proto.AppID) {
	switch appID {
	case proto.AppVi:
		if !s.viRunning {
			return
		}
		s.viRunning = false
		_ = ctx.SendToCapResult(s.viCap, uint16(proto.MsgAppShutdown), nil, kernel.Capability{})

	case proto.AppMC:
		if !s.mcRunning {
			return
		}
		s.mcRunning = false
		_ = ctx.SendToCapResult(s.mcCap, uint16(proto.MsgAppShutdown), nil, kernel.Capability{})

	case proto.AppVector:
		if !s.vectorRunning {
			return
		}
		s.vectorRunning = false
		_ = ctx.SendToCapResult(s.vectorCap, uint16(proto.MsgAppShutdown), nil, kernel.Capability{})
	}
}

func (s *Service) appCapByID(appID proto.AppID) kernel.Capability {
	switch appID {
	case proto.AppVi:
		return s.viCap
	case proto.AppMC:
		return s.mcCap
	case proto.AppVector:
		return s.vectorCap
	default:
		return kernel.Capability{}
	}
}
