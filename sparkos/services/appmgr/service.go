package appmgr

import (
	"sync"

	"spark/hal"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"
	hexedittask "spark/sparkos/tasks/hexedit"
	imgviewtask "spark/sparkos/tasks/imgview"
	mctask "spark/sparkos/tasks/mc"
	rtdemotask "spark/sparkos/tasks/rtdemo"
	rtvoxeltask "spark/sparkos/tasks/rtvoxel"
	snaketask "spark/sparkos/tasks/snake"
	vectortask "spark/sparkos/tasks/vector"
	vitask "spark/sparkos/tasks/vi"
)

// autoUnloadAfterTicks controls how long an app may stay inactive before being shutdown.
//
// The tick duration is platform-defined, but on TinyGo builds it is currently 1ms.
const autoUnloadAfterTicks = 30_000

type Service struct {
	disp   hal.Display
	vfsCap kernel.Capability

	rtdemoProxyCap  kernel.Capability
	rtvoxelProxyCap kernel.Capability
	imgviewProxyCap kernel.Capability
	hexProxyCap     kernel.Capability
	snakeProxyCap   kernel.Capability
	viProxyCap      kernel.Capability
	mcProxyCap      kernel.Capability
	vectorProxyCap  kernel.Capability

	rtdemoCap  kernel.Capability
	rtvoxelCap kernel.Capability
	imgviewCap kernel.Capability
	hexCap     kernel.Capability
	snakeCap   kernel.Capability
	viCap      kernel.Capability
	mcCap      kernel.Capability
	vectorCap  kernel.Capability

	rtdemoEP  kernel.Capability
	rtvoxelEP kernel.Capability
	imgviewEP kernel.Capability
	hexEP     kernel.Capability
	snakeEP   kernel.Capability
	viEP      kernel.Capability
	mcEP      kernel.Capability
	vectorEP  kernel.Capability

	mu sync.Mutex

	rtdemoRunning  bool
	rtvoxelRunning bool
	imgviewRunning bool
	hexRunning     bool
	snakeRunning   bool
	viRunning      bool
	mcRunning      bool
	vectorRunning  bool

	rtdemoActive  bool
	rtvoxelActive bool
	imgviewActive bool
	hexActive     bool
	snakeActive   bool
	viActive      bool
	mcActive      bool
	vectorActive  bool

	rtdemoInactiveSince  uint64
	rtvoxelInactiveSince uint64
	imgviewInactiveSince uint64
	hexInactiveSince     uint64
	snakeInactiveSince   uint64
	viInactiveSince      uint64
	mcInactiveSince      uint64
	vectorInactiveSince  uint64
}

func New(disp hal.Display, vfsCap, rtdemoProxyCap, rtvoxelProxyCap, imgviewProxyCap, hexProxyCap, snakeProxyCap, viProxyCap, mcProxyCap, vectorProxyCap, rtdemoCap, rtvoxelCap, imgviewCap, hexCap, snakeCap, viCap, mcCap, vectorCap, rtdemoEP, rtvoxelEP, imgviewEP, hexEP, snakeEP, viEP, mcEP, vectorEP kernel.Capability) *Service {
	return &Service{
		disp:            disp,
		vfsCap:          vfsCap,
		rtdemoProxyCap:  rtdemoProxyCap,
		rtvoxelProxyCap: rtvoxelProxyCap,
		imgviewProxyCap: imgviewProxyCap,
		hexProxyCap:     hexProxyCap,
		snakeProxyCap:   snakeProxyCap,
		viProxyCap:      viProxyCap,
		mcProxyCap:      mcProxyCap,
		vectorProxyCap:  vectorProxyCap,
		rtdemoCap:       rtdemoCap,
		rtvoxelCap:      rtvoxelCap,
		imgviewCap:      imgviewCap,
		hexCap:          hexCap,
		snakeCap:        snakeCap,
		viCap:           viCap,
		mcCap:           mcCap,
		vectorCap:       vectorCap,
		rtdemoEP:        rtdemoEP,
		rtvoxelEP:       rtvoxelEP,
		imgviewEP:       imgviewEP,
		hexEP:           hexEP,
		snakeEP:         snakeEP,
		viEP:            viEP,
		mcEP:            mcEP,
		vectorEP:        vectorEP,
	}
}

func (s *Service) Run(ctx *kernel.Context) {
	go s.watchdog(ctx)
	go s.runProxy(ctx, s.rtdemoProxyCap, proto.AppRTDemo)
	go s.runProxy(ctx, s.rtvoxelProxyCap, proto.AppRTVoxel)
	go s.runProxy(ctx, s.imgviewProxyCap, proto.AppImgView)
	go s.runProxy(ctx, s.hexProxyCap, proto.AppHex)
	go s.runProxy(ctx, s.snakeProxyCap, proto.AppSnake)
	go s.runProxy(ctx, s.viProxyCap, proto.AppVi)
	go s.runProxy(ctx, s.mcProxyCap, proto.AppMC)
	go s.runProxy(ctx, s.vectorProxyCap, proto.AppVector)
	select {}
}

func (s *Service) watchdog(ctx *kernel.Context) {
	if autoUnloadAfterTicks == 0 {
		return
	}

	last := ctx.NowTick()
	for {
		last = ctx.WaitTick(last)
		s.shutdownIdle(ctx, last)
	}
}

func (s *Service) shutdownIdle(ctx *kernel.Context, now uint64) {
	var stop []proto.AppID

	s.mu.Lock()
	stop = s.appendStopIfIdle(stop, proto.AppRTDemo, s.rtdemoRunning, s.rtdemoActive, s.rtdemoInactiveSince, now)
	stop = s.appendStopIfIdle(stop, proto.AppRTVoxel, s.rtvoxelRunning, s.rtvoxelActive, s.rtvoxelInactiveSince, now)
	stop = s.appendStopIfIdle(stop, proto.AppImgView, s.imgviewRunning, s.imgviewActive, s.imgviewInactiveSince, now)
	stop = s.appendStopIfIdle(stop, proto.AppHex, s.hexRunning, s.hexActive, s.hexInactiveSince, now)
	stop = s.appendStopIfIdle(stop, proto.AppSnake, s.snakeRunning, s.snakeActive, s.snakeInactiveSince, now)
	stop = s.appendStopIfIdle(stop, proto.AppVi, s.viRunning, s.viActive, s.viInactiveSince, now)
	stop = s.appendStopIfIdle(stop, proto.AppMC, s.mcRunning, s.mcActive, s.mcInactiveSince, now)
	stop = s.appendStopIfIdle(stop, proto.AppVector, s.vectorRunning, s.vectorActive, s.vectorInactiveSince, now)
	s.mu.Unlock()

	for _, id := range stop {
		s.stop(ctx, id)
	}
}

func (s *Service) appendStopIfIdle(out []proto.AppID, appID proto.AppID, running, active bool, inactiveSince, now uint64) []proto.AppID {
	if !running || active || inactiveSince == 0 {
		return out
	}
	if now-inactiveSince < autoUnloadAfterTicks {
		return out
	}
	return append(out, appID)
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
				now := ctx.NowTick()
				s.ensureRunning(ctx, appID)
				s.setActive(appID, true, now)
				_ = ctx.SendToCapResult(s.appCapByID(appID), msg.Kind, msg.Data[:msg.Len], msg.Cap)
				continue
			}

			s.setActive(appID, false, ctx.NowTick())
			if s.isRunning(appID) {
				_ = ctx.SendToCapResult(s.appCapByID(appID), msg.Kind, msg.Data[:msg.Len], kernel.Capability{})
			}

		case proto.MsgTermInput:
			s.ensureRunning(ctx, appID)
			_ = ctx.SendToCapResult(s.appCapByID(appID), msg.Kind, msg.Data[:msg.Len], kernel.Capability{})
		}
	}
}

func (s *Service) ensureRunning(ctx *kernel.Context, appID proto.AppID) {
	s.mu.Lock()
	running := s.isRunningLocked(appID)
	s.mu.Unlock()
	if running {
		return
	}

	switch appID {
	case proto.AppRTDemo:
		ctx.AddTask(rtdemotask.New(s.disp, s.rtdemoEP))
		s.mu.Lock()
		s.rtdemoRunning = true
		s.mu.Unlock()

	case proto.AppRTVoxel:
		ctx.AddTask(rtvoxeltask.New(s.disp, s.rtvoxelEP))
		s.mu.Lock()
		s.rtvoxelRunning = true
		s.mu.Unlock()

	case proto.AppImgView:
		ctx.AddTask(imgviewtask.New(s.disp, s.imgviewEP, s.vfsCap))
		s.mu.Lock()
		s.imgviewRunning = true
		s.mu.Unlock()

	case proto.AppHex:
		ctx.AddTask(hexedittask.New(s.disp, s.hexEP, s.vfsCap))
		s.mu.Lock()
		s.hexRunning = true
		s.mu.Unlock()

	case proto.AppSnake:
		ctx.AddTask(snaketask.New(s.disp, s.snakeEP))
		s.mu.Lock()
		s.snakeRunning = true
		s.mu.Unlock()

	case proto.AppVi:
		ctx.AddTask(vitask.New(s.disp, s.viEP, s.vfsCap))
		s.mu.Lock()
		s.viRunning = true
		s.mu.Unlock()

	case proto.AppMC:
		ctx.AddTask(mctask.New(s.disp, s.mcEP, s.vfsCap))
		s.mu.Lock()
		s.mcRunning = true
		s.mu.Unlock()

	case proto.AppVector:
		ctx.AddTask(vectortask.New(s.disp, s.vectorEP))
		s.mu.Lock()
		s.vectorRunning = true
		s.mu.Unlock()
	}
}

func (s *Service) stop(ctx *kernel.Context, appID proto.AppID) {
	s.mu.Lock()
	running := s.isRunningLocked(appID)
	if running {
		s.setRunningLocked(appID, false)
		s.setActiveLocked(appID, false, ctx.NowTick())
	}
	s.mu.Unlock()
	if !running {
		return
	}

	switch appID {
	case proto.AppRTDemo:
		_ = ctx.SendToCapResult(s.rtdemoCap, uint16(proto.MsgAppShutdown), nil, kernel.Capability{})

	case proto.AppRTVoxel:
		_ = ctx.SendToCapResult(s.rtvoxelCap, uint16(proto.MsgAppShutdown), nil, kernel.Capability{})

	case proto.AppImgView:
		_ = ctx.SendToCapResult(s.imgviewCap, uint16(proto.MsgAppShutdown), nil, kernel.Capability{})

	case proto.AppHex:
		_ = ctx.SendToCapResult(s.hexCap, uint16(proto.MsgAppShutdown), nil, kernel.Capability{})

	case proto.AppSnake:
		_ = ctx.SendToCapResult(s.snakeCap, uint16(proto.MsgAppShutdown), nil, kernel.Capability{})

	case proto.AppVi:
		_ = ctx.SendToCapResult(s.viCap, uint16(proto.MsgAppShutdown), nil, kernel.Capability{})

	case proto.AppMC:
		_ = ctx.SendToCapResult(s.mcCap, uint16(proto.MsgAppShutdown), nil, kernel.Capability{})

	case proto.AppVector:
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
	case proto.AppSnake:
		return s.snakeCap
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
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isRunningLocked(appID)
}

func (s *Service) isRunningLocked(appID proto.AppID) bool {
	switch appID {
	case proto.AppRTDemo:
		return s.rtdemoRunning
	case proto.AppRTVoxel:
		return s.rtvoxelRunning
	case proto.AppImgView:
		return s.imgviewRunning
	case proto.AppHex:
		return s.hexRunning
	case proto.AppSnake:
		return s.snakeRunning
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

func (s *Service) setRunningLocked(appID proto.AppID, running bool) {
	switch appID {
	case proto.AppRTDemo:
		s.rtdemoRunning = running
	case proto.AppRTVoxel:
		s.rtvoxelRunning = running
	case proto.AppImgView:
		s.imgviewRunning = running
	case proto.AppHex:
		s.hexRunning = running
	case proto.AppSnake:
		s.snakeRunning = running
	case proto.AppVi:
		s.viRunning = running
	case proto.AppMC:
		s.mcRunning = running
	case proto.AppVector:
		s.vectorRunning = running
	}
}

func (s *Service) setActive(appID proto.AppID, active bool, now uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setActiveLocked(appID, active, now)
}

func (s *Service) setActiveLocked(appID proto.AppID, active bool, now uint64) {
	switch appID {
	case proto.AppRTDemo:
		s.rtdemoActive = active
		s.rtdemoInactiveSince = inactiveSince(active, now, s.rtdemoInactiveSince)
	case proto.AppRTVoxel:
		s.rtvoxelActive = active
		s.rtvoxelInactiveSince = inactiveSince(active, now, s.rtvoxelInactiveSince)
	case proto.AppImgView:
		s.imgviewActive = active
		s.imgviewInactiveSince = inactiveSince(active, now, s.imgviewInactiveSince)
	case proto.AppHex:
		s.hexActive = active
		s.hexInactiveSince = inactiveSince(active, now, s.hexInactiveSince)
	case proto.AppSnake:
		s.snakeActive = active
		s.snakeInactiveSince = inactiveSince(active, now, s.snakeInactiveSince)
	case proto.AppVi:
		s.viActive = active
		s.viInactiveSince = inactiveSince(active, now, s.viInactiveSince)
	case proto.AppMC:
		s.mcActive = active
		s.mcInactiveSince = inactiveSince(active, now, s.mcInactiveSince)
	case proto.AppVector:
		s.vectorActive = active
		s.vectorInactiveSince = inactiveSince(active, now, s.vectorInactiveSince)
	}
}

func inactiveSince(active bool, now, prev uint64) uint64 {
	if active {
		return 0
	}
	if prev != 0 {
		return prev
	}
	return now
}
