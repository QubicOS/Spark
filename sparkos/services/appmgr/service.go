package appmgr

import (
	"sync"

	"spark/hal"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"
	archivetask "spark/sparkos/tasks/archive"
	basictask "spark/sparkos/tasks/basic"
	calendartask "spark/sparkos/tasks/calendar"
	fbtesttask "spark/sparkos/tasks/fbtest"
	gpioscopetask "spark/sparkos/tasks/gpioscope"
	hexedittask "spark/sparkos/tasks/hexedit"
	imgviewtask "spark/sparkos/tasks/imgview"
	mctask "spark/sparkos/tasks/mc"
	quarkdonuttask "spark/sparkos/tasks/quarkdonut"
	rfanalyzertask "spark/sparkos/tasks/rfanalyzer"
	rtdemotask "spark/sparkos/tasks/rtdemo"
	rtvoxeltask "spark/sparkos/tasks/rtvoxel"
	serialtermtask "spark/sparkos/tasks/serialterm"
	snaketask "spark/sparkos/tasks/snake"
	teaplayertask "spark/sparkos/tasks/teaplayer"
	tetristask "spark/sparkos/tasks/tetris"
	todotask "spark/sparkos/tasks/todo"
	userstask "spark/sparkos/tasks/users"
	vectortask "spark/sparkos/tasks/vector"
	vitask "spark/sparkos/tasks/vi"
)

// autoUnloadAfterTicks controls how long an app may stay inactive before being shutdown.
//
// The tick duration is platform-defined, but on TinyGo builds it is currently 1ms.
const autoUnloadAfterTicks = 30_000

type Service struct {
	disp      hal.Display
	vfsCap    kernel.Capability
	audioCap  kernel.Capability
	timeCap   kernel.Capability
	gpioCap   kernel.Capability
	serialCap kernel.Capability

	rtdemoProxyCap     kernel.Capability
	rtvoxelProxyCap    kernel.Capability
	imgviewProxyCap    kernel.Capability
	hexProxyCap        kernel.Capability
	snakeProxyCap      kernel.Capability
	tetrisProxyCap     kernel.Capability
	calendarProxyCap   kernel.Capability
	todoProxyCap       kernel.Capability
	archiveProxyCap    kernel.Capability
	viProxyCap         kernel.Capability
	mcProxyCap         kernel.Capability
	vectorProxyCap     kernel.Capability
	teaProxyCap        kernel.Capability
	basicProxyCap      kernel.Capability
	rfAnalyzerProxyCap kernel.Capability
	gpioscopeProxyCap  kernel.Capability
	fbtestProxyCap     kernel.Capability
	serialtermProxyCap kernel.Capability
	usersProxyCap      kernel.Capability
	donutProxyCap      kernel.Capability

	rtdemoCap     kernel.Capability
	rtvoxelCap    kernel.Capability
	imgviewCap    kernel.Capability
	hexCap        kernel.Capability
	snakeCap      kernel.Capability
	tetrisCap     kernel.Capability
	calendarCap   kernel.Capability
	todoCap       kernel.Capability
	archiveCap    kernel.Capability
	viCap         kernel.Capability
	mcCap         kernel.Capability
	vectorCap     kernel.Capability
	teaCap        kernel.Capability
	basicCap      kernel.Capability
	rfAnalyzerCap kernel.Capability
	gpioscopeCap  kernel.Capability
	fbtestCap     kernel.Capability
	serialtermCap kernel.Capability
	usersCap      kernel.Capability
	donutCap      kernel.Capability

	rtdemoEP     kernel.Capability
	rtvoxelEP    kernel.Capability
	imgviewEP    kernel.Capability
	hexEP        kernel.Capability
	snakeEP      kernel.Capability
	tetrisEP     kernel.Capability
	calendarEP   kernel.Capability
	todoEP       kernel.Capability
	archiveEP    kernel.Capability
	viEP         kernel.Capability
	mcEP         kernel.Capability
	vectorEP     kernel.Capability
	teaEP        kernel.Capability
	basicEP      kernel.Capability
	rfAnalyzerEP kernel.Capability
	gpioscopeEP  kernel.Capability
	fbtestEP     kernel.Capability
	serialtermEP kernel.Capability
	usersEP      kernel.Capability
	donutEP      kernel.Capability

	mu sync.Mutex

	rtdemoRunning     bool
	rtvoxelRunning    bool
	imgviewRunning    bool
	hexRunning        bool
	snakeRunning      bool
	tetrisRunning     bool
	calendarRunning   bool
	todoRunning       bool
	archiveRunning    bool
	viRunning         bool
	mcRunning         bool
	vectorRunning     bool
	teaRunning        bool
	basicRunning      bool
	rfAnalyzerRunning bool
	gpioscopeRunning  bool
	fbtestRunning     bool
	serialtermRunning bool
	usersRunning      bool
	donutRunning      bool

	rtdemoActive     bool
	rtvoxelActive    bool
	imgviewActive    bool
	hexActive        bool
	snakeActive      bool
	tetrisActive     bool
	calendarActive   bool
	todoActive       bool
	archiveActive    bool
	viActive         bool
	mcActive         bool
	vectorActive     bool
	teaActive        bool
	basicActive      bool
	rfAnalyzerActive bool
	gpioscopeActive  bool
	fbtestActive     bool
	serialtermActive bool
	usersActive      bool
	donutActive      bool

	rtdemoInactiveSince     uint64
	rtvoxelInactiveSince    uint64
	imgviewInactiveSince    uint64
	hexInactiveSince        uint64
	snakeInactiveSince      uint64
	tetrisInactiveSince     uint64
	calendarInactiveSince   uint64
	todoInactiveSince       uint64
	archiveInactiveSince    uint64
	viInactiveSince         uint64
	mcInactiveSince         uint64
	vectorInactiveSince     uint64
	teaInactiveSince        uint64
	basicInactiveSince      uint64
	rfAnalyzerInactiveSince uint64
	gpioscopeInactiveSince  uint64
	fbtestInactiveSince     uint64
	serialtermInactiveSince uint64
	usersInactiveSince      uint64
	donutInactiveSince      uint64
}

func New(disp hal.Display, vfsCap, audioCap, timeCap, gpioCap, serialCap, rtdemoProxyCap, rtvoxelProxyCap, imgviewProxyCap, hexProxyCap, snakeProxyCap, tetrisProxyCap, calendarProxyCap, todoProxyCap, archiveProxyCap, viProxyCap, mcProxyCap, vectorProxyCap, teaProxyCap, basicProxyCap, rfAnalyzerProxyCap, gpioscopeProxyCap, fbtestProxyCap, serialtermProxyCap, usersProxyCap, donutProxyCap, rtdemoCap, rtvoxelCap, imgviewCap, hexCap, snakeCap, tetrisCap, calendarCap, todoCap, archiveCap, viCap, mcCap, vectorCap, teaCap, basicCap, rfAnalyzerCap, gpioscopeCap, fbtestCap, serialtermCap, usersCap, donutCap, rtdemoEP, rtvoxelEP, imgviewEP, hexEP, snakeEP, tetrisEP, calendarEP, todoEP, archiveEP, viEP, mcEP, vectorEP, teaEP, basicEP, rfAnalyzerEP, gpioscopeEP, fbtestEP, serialtermEP, usersEP, donutEP kernel.Capability) *Service {
	return &Service{
		disp:               disp,
		vfsCap:             vfsCap,
		audioCap:           audioCap,
		timeCap:            timeCap,
		gpioCap:            gpioCap,
		serialCap:          serialCap,
		rtdemoProxyCap:     rtdemoProxyCap,
		rtvoxelProxyCap:    rtvoxelProxyCap,
		imgviewProxyCap:    imgviewProxyCap,
		hexProxyCap:        hexProxyCap,
		snakeProxyCap:      snakeProxyCap,
		tetrisProxyCap:     tetrisProxyCap,
		calendarProxyCap:   calendarProxyCap,
		todoProxyCap:       todoProxyCap,
		archiveProxyCap:    archiveProxyCap,
		viProxyCap:         viProxyCap,
		mcProxyCap:         mcProxyCap,
		vectorProxyCap:     vectorProxyCap,
		teaProxyCap:        teaProxyCap,
		basicProxyCap:      basicProxyCap,
		rfAnalyzerProxyCap: rfAnalyzerProxyCap,
		gpioscopeProxyCap:  gpioscopeProxyCap,
		fbtestProxyCap:     fbtestProxyCap,
		serialtermProxyCap: serialtermProxyCap,
		usersProxyCap:      usersProxyCap,
		donutProxyCap:      donutProxyCap,
		rtdemoCap:          rtdemoCap,
		rtvoxelCap:         rtvoxelCap,
		imgviewCap:         imgviewCap,
		hexCap:             hexCap,
		snakeCap:           snakeCap,
		tetrisCap:          tetrisCap,
		calendarCap:        calendarCap,
		todoCap:            todoCap,
		archiveCap:         archiveCap,
		viCap:              viCap,
		mcCap:              mcCap,
		vectorCap:          vectorCap,
		teaCap:             teaCap,
		basicCap:           basicCap,
		rfAnalyzerCap:      rfAnalyzerCap,
		gpioscopeCap:       gpioscopeCap,
		fbtestCap:          fbtestCap,
		serialtermCap:      serialtermCap,
		usersCap:           usersCap,
		donutCap:           donutCap,
		rtdemoEP:           rtdemoEP,
		rtvoxelEP:          rtvoxelEP,
		imgviewEP:          imgviewEP,
		hexEP:              hexEP,
		snakeEP:            snakeEP,
		tetrisEP:           tetrisEP,
		calendarEP:         calendarEP,
		todoEP:             todoEP,
		archiveEP:          archiveEP,
		viEP:               viEP,
		mcEP:               mcEP,
		vectorEP:           vectorEP,
		teaEP:              teaEP,
		basicEP:            basicEP,
		rfAnalyzerEP:       rfAnalyzerEP,
		gpioscopeEP:        gpioscopeEP,
		fbtestEP:           fbtestEP,
		serialtermEP:       serialtermEP,
		usersEP:            usersEP,
		donutEP:            donutEP,
	}
}

func (s *Service) Run(ctx *kernel.Context) {
	go s.watchdog(ctx)
	go s.runProxy(ctx, s.rtdemoProxyCap, proto.AppRTDemo)
	go s.runProxy(ctx, s.rtvoxelProxyCap, proto.AppRTVoxel)
	go s.runProxy(ctx, s.imgviewProxyCap, proto.AppImgView)
	go s.runProxy(ctx, s.hexProxyCap, proto.AppHex)
	go s.runProxy(ctx, s.snakeProxyCap, proto.AppSnake)
	go s.runProxy(ctx, s.tetrisProxyCap, proto.AppTetris)
	go s.runProxy(ctx, s.calendarProxyCap, proto.AppCalendar)
	go s.runProxy(ctx, s.todoProxyCap, proto.AppTodo)
	go s.runProxy(ctx, s.archiveProxyCap, proto.AppArchive)
	go s.runProxy(ctx, s.viProxyCap, proto.AppVi)
	go s.runProxy(ctx, s.mcProxyCap, proto.AppMC)
	go s.runProxy(ctx, s.vectorProxyCap, proto.AppVector)
	go s.runProxy(ctx, s.teaProxyCap, proto.AppTEA)
	go s.runProxy(ctx, s.basicProxyCap, proto.AppBasic)
	go s.runProxy(ctx, s.rfAnalyzerProxyCap, proto.AppRFAnalyzer)
	go s.runProxy(ctx, s.gpioscopeProxyCap, proto.AppGPIOScope)
	go s.runProxy(ctx, s.fbtestProxyCap, proto.AppFBTest)
	go s.runProxy(ctx, s.serialtermProxyCap, proto.AppSerialTerm)
	go s.runProxy(ctx, s.usersProxyCap, proto.AppUsers)
	go s.runProxy(ctx, s.donutProxyCap, proto.AppQuarkDonut)
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
	stop = s.appendStopIfIdle(stop, proto.AppTetris, s.tetrisRunning, s.tetrisActive, s.tetrisInactiveSince, now)
	stop = s.appendStopIfIdle(stop, proto.AppCalendar, s.calendarRunning, s.calendarActive, s.calendarInactiveSince, now)
	stop = s.appendStopIfIdle(stop, proto.AppTodo, s.todoRunning, s.todoActive, s.todoInactiveSince, now)
	stop = s.appendStopIfIdle(stop, proto.AppArchive, s.archiveRunning, s.archiveActive, s.archiveInactiveSince, now)
	stop = s.appendStopIfIdle(stop, proto.AppVi, s.viRunning, s.viActive, s.viInactiveSince, now)
	stop = s.appendStopIfIdle(stop, proto.AppMC, s.mcRunning, s.mcActive, s.mcInactiveSince, now)
	stop = s.appendStopIfIdle(stop, proto.AppVector, s.vectorRunning, s.vectorActive, s.vectorInactiveSince, now)
	stop = s.appendStopIfIdle(stop, proto.AppTEA, s.teaRunning, s.teaActive, s.teaInactiveSince, now)
	stop = s.appendStopIfIdle(stop, proto.AppBasic, s.basicRunning, s.basicActive, s.basicInactiveSince, now)
	stop = s.appendStopIfIdle(stop, proto.AppRFAnalyzer, s.rfAnalyzerRunning, s.rfAnalyzerActive, s.rfAnalyzerInactiveSince, now)
	stop = s.appendStopIfIdle(stop, proto.AppGPIOScope, s.gpioscopeRunning, s.gpioscopeActive, s.gpioscopeInactiveSince, now)
	stop = s.appendStopIfIdle(stop, proto.AppFBTest, s.fbtestRunning, s.fbtestActive, s.fbtestInactiveSince, now)
	stop = s.appendStopIfIdle(stop, proto.AppSerialTerm, s.serialtermRunning, s.serialtermActive, s.serialtermInactiveSince, now)
	stop = s.appendStopIfIdle(stop, proto.AppUsers, s.usersRunning, s.usersActive, s.usersInactiveSince, now)
	stop = s.appendStopIfIdle(stop, proto.AppQuarkDonut, s.donutRunning, s.donutActive, s.donutInactiveSince, now)
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
			_ = ctx.SendToCapRetry(
				s.appCapByID(appID),
				msg.Kind,
				msg.Payload(),
				kernel.Capability{},
				proxySendRetryLimit,
			)

		case proto.MsgAppControl:
			active, ok := proto.DecodeAppControlPayload(msg.Payload())
			if !ok {
				continue
			}
			if active {
				now := ctx.NowTick()
				s.ensureRunning(ctx, appID)
				s.setActive(appID, true, now)
				_ = ctx.SendToCapRetry(s.appCapByID(appID), msg.Kind, msg.Payload(), msg.Cap, proxySendRetryLimit)
				continue
			}

			s.setActive(appID, false, ctx.NowTick())
			if s.isRunning(appID) {
				_ = ctx.SendToCapRetry(
					s.appCapByID(appID),
					msg.Kind,
					msg.Payload(),
					kernel.Capability{},
					proxySendRetryLimit,
				)
			}

		case proto.MsgTermInput:
			s.ensureRunning(ctx, appID)
			_ = ctx.SendToCapRetry(
				s.appCapByID(appID),
				msg.Kind,
				msg.Payload(),
				kernel.Capability{},
				proxySendRetryLimit,
			)
		}
	}
}

const proxySendRetryLimit = 50

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

	case proto.AppTetris:
		ctx.AddTask(tetristask.New(s.disp, s.tetrisEP))
		s.mu.Lock()
		s.tetrisRunning = true
		s.mu.Unlock()

	case proto.AppCalendar:
		ctx.AddTask(calendartask.New(s.disp, s.calendarEP, s.vfsCap))
		s.mu.Lock()
		s.calendarRunning = true
		s.mu.Unlock()

	case proto.AppTodo:
		ctx.AddTask(todotask.New(s.disp, s.todoEP, s.vfsCap))
		s.mu.Lock()
		s.todoRunning = true
		s.mu.Unlock()

	case proto.AppArchive:
		ctx.AddTask(archivetask.New(s.disp, s.archiveEP, s.vfsCap))
		s.mu.Lock()
		s.archiveRunning = true
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
		ctx.AddTask(vectortask.New(s.disp, s.vectorEP, s.vfsCap))
		s.mu.Lock()
		s.vectorRunning = true
		s.mu.Unlock()

	case proto.AppTEA:
		ctx.AddTask(teaplayertask.New(s.disp, s.teaEP, s.vfsCap, s.audioCap))
		s.mu.Lock()
		s.teaRunning = true
		s.mu.Unlock()

	case proto.AppBasic:
		ctx.AddTask(basictask.New(s.disp, s.basicEP, s.vfsCap))
		s.mu.Lock()
		s.basicRunning = true
		s.mu.Unlock()

	case proto.AppRFAnalyzer:
		ctx.AddTask(rfanalyzertask.New(s.disp, s.rfAnalyzerEP, s.vfsCap))
		s.mu.Lock()
		s.rfAnalyzerRunning = true
		s.mu.Unlock()

	case proto.AppGPIOScope:
		ctx.AddTask(gpioscopetask.New(s.disp, s.gpioscopeEP, s.timeCap, s.gpioCap))
		s.mu.Lock()
		s.gpioscopeRunning = true
		s.mu.Unlock()

	case proto.AppFBTest:
		ctx.AddTask(fbtesttask.New(s.disp, s.fbtestEP))
		s.mu.Lock()
		s.fbtestRunning = true
		s.mu.Unlock()

	case proto.AppSerialTerm:
		ctx.AddTask(serialtermtask.New(s.disp, s.serialCap, s.serialtermEP))
		s.mu.Lock()
		s.serialtermRunning = true
		s.mu.Unlock()

	case proto.AppUsers:
		ctx.AddTask(userstask.New(s.disp, s.usersEP, s.vfsCap))
		s.mu.Lock()
		s.usersRunning = true
		s.mu.Unlock()

	case proto.AppQuarkDonut:
		ctx.AddTask(quarkdonuttask.New(s.disp, s.donutEP))
		s.mu.Lock()
		s.donutRunning = true
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

	case proto.AppTetris:
		_ = ctx.SendToCapResult(s.tetrisCap, uint16(proto.MsgAppShutdown), nil, kernel.Capability{})

	case proto.AppCalendar:
		_ = ctx.SendToCapResult(s.calendarCap, uint16(proto.MsgAppShutdown), nil, kernel.Capability{})

	case proto.AppTodo:
		_ = ctx.SendToCapResult(s.todoCap, uint16(proto.MsgAppShutdown), nil, kernel.Capability{})

	case proto.AppArchive:
		_ = ctx.SendToCapResult(s.archiveCap, uint16(proto.MsgAppShutdown), nil, kernel.Capability{})

	case proto.AppVi:
		_ = ctx.SendToCapResult(s.viCap, uint16(proto.MsgAppShutdown), nil, kernel.Capability{})

	case proto.AppMC:
		_ = ctx.SendToCapResult(s.mcCap, uint16(proto.MsgAppShutdown), nil, kernel.Capability{})

	case proto.AppVector:
		_ = ctx.SendToCapResult(s.vectorCap, uint16(proto.MsgAppShutdown), nil, kernel.Capability{})

	case proto.AppTEA:
		_ = ctx.SendToCapResult(s.teaCap, uint16(proto.MsgAppShutdown), nil, kernel.Capability{})

	case proto.AppBasic:
		_ = ctx.SendToCapResult(s.basicCap, uint16(proto.MsgAppShutdown), nil, kernel.Capability{})

	case proto.AppRFAnalyzer:
		_ = ctx.SendToCapResult(s.rfAnalyzerCap, uint16(proto.MsgAppShutdown), nil, kernel.Capability{})
	case proto.AppGPIOScope:
		_ = ctx.SendToCapResult(s.gpioscopeCap, uint16(proto.MsgAppShutdown), nil, kernel.Capability{})

	case proto.AppFBTest:
		_ = ctx.SendToCapResult(s.fbtestCap, uint16(proto.MsgAppShutdown), nil, kernel.Capability{})

	case proto.AppSerialTerm:
		_ = ctx.SendToCapResult(s.serialtermCap, uint16(proto.MsgAppShutdown), nil, kernel.Capability{})

	case proto.AppUsers:
		_ = ctx.SendToCapResult(s.usersCap, uint16(proto.MsgAppShutdown), nil, kernel.Capability{})

	case proto.AppQuarkDonut:
		_ = ctx.SendToCapResult(s.donutCap, uint16(proto.MsgAppShutdown), nil, kernel.Capability{})
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
	case proto.AppTetris:
		return s.tetrisCap
	case proto.AppCalendar:
		return s.calendarCap
	case proto.AppTodo:
		return s.todoCap
	case proto.AppArchive:
		return s.archiveCap
	case proto.AppVi:
		return s.viCap
	case proto.AppMC:
		return s.mcCap
	case proto.AppVector:
		return s.vectorCap
	case proto.AppTEA:
		return s.teaCap
	case proto.AppBasic:
		return s.basicCap
	case proto.AppRFAnalyzer:
		return s.rfAnalyzerCap
	case proto.AppGPIOScope:
		return s.gpioscopeCap
	case proto.AppFBTest:
		return s.fbtestCap
	case proto.AppSerialTerm:
		return s.serialtermCap
	case proto.AppUsers:
		return s.usersCap
	case proto.AppQuarkDonut:
		return s.donutCap
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
	case proto.AppTetris:
		return s.tetrisRunning
	case proto.AppCalendar:
		return s.calendarRunning
	case proto.AppTodo:
		return s.todoRunning
	case proto.AppArchive:
		return s.archiveRunning
	case proto.AppVi:
		return s.viRunning
	case proto.AppMC:
		return s.mcRunning
	case proto.AppVector:
		return s.vectorRunning
	case proto.AppTEA:
		return s.teaRunning
	case proto.AppBasic:
		return s.basicRunning
	case proto.AppRFAnalyzer:
		return s.rfAnalyzerRunning
	case proto.AppGPIOScope:
		return s.gpioscopeRunning
	case proto.AppFBTest:
		return s.fbtestRunning
	case proto.AppSerialTerm:
		return s.serialtermRunning
	case proto.AppUsers:
		return s.usersRunning
	case proto.AppQuarkDonut:
		return s.donutRunning
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
	case proto.AppTetris:
		s.tetrisRunning = running
	case proto.AppCalendar:
		s.calendarRunning = running
	case proto.AppTodo:
		s.todoRunning = running
	case proto.AppArchive:
		s.archiveRunning = running
	case proto.AppVi:
		s.viRunning = running
	case proto.AppMC:
		s.mcRunning = running
	case proto.AppVector:
		s.vectorRunning = running
	case proto.AppTEA:
		s.teaRunning = running
	case proto.AppBasic:
		s.basicRunning = running
	case proto.AppRFAnalyzer:
		s.rfAnalyzerRunning = running
	case proto.AppGPIOScope:
		s.gpioscopeRunning = running
	case proto.AppFBTest:
		s.fbtestRunning = running
	case proto.AppSerialTerm:
		s.serialtermRunning = running
	case proto.AppUsers:
		s.usersRunning = running
	case proto.AppQuarkDonut:
		s.donutRunning = running
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
	case proto.AppTetris:
		s.tetrisActive = active
		s.tetrisInactiveSince = inactiveSince(active, now, s.tetrisInactiveSince)
	case proto.AppCalendar:
		s.calendarActive = active
		s.calendarInactiveSince = inactiveSince(active, now, s.calendarInactiveSince)
	case proto.AppTodo:
		s.todoActive = active
		s.todoInactiveSince = inactiveSince(active, now, s.todoInactiveSince)
	case proto.AppArchive:
		s.archiveActive = active
		s.archiveInactiveSince = inactiveSince(active, now, s.archiveInactiveSince)
	case proto.AppVi:
		s.viActive = active
		s.viInactiveSince = inactiveSince(active, now, s.viInactiveSince)
	case proto.AppMC:
		s.mcActive = active
		s.mcInactiveSince = inactiveSince(active, now, s.mcInactiveSince)
	case proto.AppVector:
		s.vectorActive = active
		s.vectorInactiveSince = inactiveSince(active, now, s.vectorInactiveSince)
	case proto.AppTEA:
		s.teaActive = active
		s.teaInactiveSince = inactiveSince(active, now, s.teaInactiveSince)
	case proto.AppBasic:
		s.basicActive = active
		s.basicInactiveSince = inactiveSince(active, now, s.basicInactiveSince)
	case proto.AppRFAnalyzer:
		s.rfAnalyzerActive = active
		s.rfAnalyzerInactiveSince = inactiveSince(active, now, s.rfAnalyzerInactiveSince)
	case proto.AppGPIOScope:
		s.gpioscopeActive = active
		s.gpioscopeInactiveSince = inactiveSince(active, now, s.gpioscopeInactiveSince)
	case proto.AppFBTest:
		s.fbtestActive = active
		s.fbtestInactiveSince = inactiveSince(active, now, s.fbtestInactiveSince)
	case proto.AppSerialTerm:
		s.serialtermActive = active
		s.serialtermInactiveSince = inactiveSince(active, now, s.serialtermInactiveSince)
	case proto.AppUsers:
		s.usersActive = active
		s.usersInactiveSince = inactiveSince(active, now, s.usersInactiveSince)
	case proto.AppQuarkDonut:
		s.donutActive = active
		s.donutInactiveSince = inactiveSince(active, now, s.donutInactiveSince)
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
