package appmgr

import (
	"spark/hal"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"
	hexedittask "spark/sparkos/tasks/hexedit"
	imgviewtask "spark/sparkos/tasks/imgview"
	mctask "spark/sparkos/tasks/mc"
	rtdemotask "spark/sparkos/tasks/rtdemo"
	rtvoxeltask "spark/sparkos/tasks/rtvoxel"
	vectortask "spark/sparkos/tasks/vector"
	vitask "spark/sparkos/tasks/vi"
)

type Service struct {
	disp   hal.Display
	vfsCap kernel.Capability

	rtdemoProxyCap  kernel.Capability
	rtvoxelProxyCap kernel.Capability
	imgviewProxyCap kernel.Capability
	hexProxyCap     kernel.Capability
	viProxyCap      kernel.Capability
	mcProxyCap      kernel.Capability
	vectorProxyCap  kernel.Capability

	rtdemoCap  kernel.Capability
	rtvoxelCap kernel.Capability
	imgviewCap kernel.Capability
	hexCap     kernel.Capability
	viCap      kernel.Capability
	mcCap      kernel.Capability
	vectorCap  kernel.Capability

	rtdemoEP  kernel.Capability
	rtvoxelEP kernel.Capability
	imgviewEP kernel.Capability
	hexEP     kernel.Capability
	viEP      kernel.Capability
	mcEP      kernel.Capability
	vectorEP  kernel.Capability

	rtdemoRunning  bool
	rtvoxelRunning bool
	imgviewRunning bool
	hexRunning     bool
	viRunning      bool
	mcRunning      bool
	vectorRunning  bool
}

func New(disp hal.Display, vfsCap, rtdemoProxyCap, rtvoxelProxyCap, imgviewProxyCap, hexProxyCap, viProxyCap, mcProxyCap, vectorProxyCap, rtdemoCap, rtvoxelCap, imgviewCap, hexCap, viCap, mcCap, vectorCap, rtdemoEP, rtvoxelEP, imgviewEP, hexEP, viEP, mcEP, vectorEP kernel.Capability) *Service {
	return &Service{
		disp:            disp,
		vfsCap:          vfsCap,
		rtdemoProxyCap:  rtdemoProxyCap,
		rtvoxelProxyCap: rtvoxelProxyCap,
		imgviewProxyCap: imgviewProxyCap,
		hexProxyCap:     hexProxyCap,
		viProxyCap:      viProxyCap,
		mcProxyCap:      mcProxyCap,
		vectorProxyCap:  vectorProxyCap,
		rtdemoCap:       rtdemoCap,
		rtvoxelCap:      rtvoxelCap,
		imgviewCap:      imgviewCap,
		hexCap:          hexCap,
		viCap:           viCap,
		mcCap:           mcCap,
		vectorCap:       vectorCap,
		rtdemoEP:        rtdemoEP,
		rtvoxelEP:       rtvoxelEP,
		imgviewEP:       imgviewEP,
		hexEP:           hexEP,
		viEP:            viEP,
		mcEP:            mcEP,
		vectorEP:        vectorEP,
	}
}

func (s *Service) Run(ctx *kernel.Context) {
	go s.runProxy(ctx, s.rtdemoProxyCap, proto.AppRTDemo)
	go s.runProxy(ctx, s.rtvoxelProxyCap, proto.AppRTVoxel)
	go s.runProxy(ctx, s.imgviewProxyCap, proto.AppImgView)
	go s.runProxy(ctx, s.hexProxyCap, proto.AppHex)
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

			if s.isRunning(appID) {
				_ = ctx.SendToCapResult(s.appCapByID(appID), msg.Kind, msg.Data[:msg.Len], kernel.Capability{})
			}
			s.stop(ctx, appID)

		case proto.MsgTermInput:
			s.ensureRunning(ctx, appID)
			_ = ctx.SendToCapResult(s.appCapByID(appID), msg.Kind, msg.Data[:msg.Len], kernel.Capability{})
		}
	}
}

func (s *Service) ensureRunning(ctx *kernel.Context, appID proto.AppID) {
	switch appID {
	case proto.AppRTDemo:
		if s.rtdemoRunning {
			return
		}
		ctx.AddTask(rtdemotask.New(s.disp, s.rtdemoEP))
		s.rtdemoRunning = true

	case proto.AppRTVoxel:
		if s.rtvoxelRunning {
			return
		}
		ctx.AddTask(rtvoxeltask.New(s.disp, s.rtvoxelEP))
		s.rtvoxelRunning = true

	case proto.AppImgView:
		if s.imgviewRunning {
			return
		}
		ctx.AddTask(imgviewtask.New(s.disp, s.imgviewEP, s.vfsCap))
		s.imgviewRunning = true

	case proto.AppHex:
		if s.hexRunning {
			return
		}
		ctx.AddTask(hexedittask.New(s.disp, s.hexEP, s.vfsCap))
		s.hexRunning = true

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
	case proto.AppRTDemo:
		if !s.rtdemoRunning {
			return
		}
		s.rtdemoRunning = false
		_ = ctx.SendToCapResult(s.rtdemoCap, uint16(proto.MsgAppShutdown), nil, kernel.Capability{})

	case proto.AppRTVoxel:
		if !s.rtvoxelRunning {
			return
		}
		s.rtvoxelRunning = false
		_ = ctx.SendToCapResult(s.rtvoxelCap, uint16(proto.MsgAppShutdown), nil, kernel.Capability{})

	case proto.AppImgView:
		if !s.imgviewRunning {
			return
		}
		s.imgviewRunning = false
		_ = ctx.SendToCapResult(s.imgviewCap, uint16(proto.MsgAppShutdown), nil, kernel.Capability{})

	case proto.AppHex:
		if !s.hexRunning {
			return
		}
		s.hexRunning = false
		_ = ctx.SendToCapResult(s.hexCap, uint16(proto.MsgAppShutdown), nil, kernel.Capability{})

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
	case proto.AppRTDemo:
		return s.rtdemoCap
	case proto.AppRTVoxel:
		return s.rtvoxelCap
	case proto.AppImgView:
		return s.imgviewCap
	case proto.AppHex:
		return s.hexCap
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

func (s *Service) isRunning(appID proto.AppID) bool {
	switch appID {
	case proto.AppRTDemo:
		return s.rtdemoRunning
	case proto.AppRTVoxel:
		return s.rtvoxelRunning
	case proto.AppImgView:
		return s.imgviewRunning
	case proto.AppHex:
		return s.hexRunning
	case proto.AppVi:
		return s.viRunning
	case proto.AppMC:
		return s.mcRunning
	case proto.AppVector:
		return s.vectorRunning
	default:
		return false
	}
}
