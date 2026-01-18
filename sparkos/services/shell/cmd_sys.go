package shell

import (
	"errors"
	"fmt"
	"runtime"
	"strconv"

	"spark/internal/buildinfo"
	consolemuxclient "spark/sparkos/client/consolemux"
	timeclient "spark/sparkos/client/time"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

func registerSysCommands(r *registry) error {
	for _, cmd := range []command{
		{Name: "ticks", Usage: "ticks", Desc: "Show current kernel tick counter.", Run: cmdTicks},
		{Name: "uptime", Usage: "uptime", Desc: "Show uptime (ticks).", Run: cmdUptime},
		{Name: "sleep", Usage: "sleep <ticks>", Desc: "Sleep for dt ticks via time service.", Run: cmdSleep},
		{Name: "version", Usage: "version", Desc: "Show build version.", Run: cmdVersion},
		{Name: "uname", Usage: "uname [-a]", Desc: "Show system information.", Run: cmdUname},
		{Name: "free", Usage: "free [-h]", Desc: "Show memory usage.", Run: cmdFree},
		{Name: "mux", Usage: "mux", Desc: "Show consolemux status (active app + focus).", Run: cmdMux},
		{Name: "focus", Usage: "focus [app|shell|toggle]", Desc: "Switch focus between shell and app.", Run: cmdFocus},
	} {
		if err := r.register(cmd); err != nil {
			return err
		}
	}
	return nil
}

func cmdTicks(ctx *kernel.Context, s *Service, _ []string, _ redirection) error {
	_ = s.printString(ctx, fmt.Sprintf("%d\n", ctx.NowTick()))
	return nil
}

func cmdUptime(ctx *kernel.Context, s *Service, _ []string, _ redirection) error {
	_ = s.printString(ctx, fmt.Sprintf("up %d ticks\n", ctx.NowTick()))
	return nil
}

func cmdSleep(ctx *kernel.Context, s *Service, args []string, _ redirection) error {
	if len(args) != 1 {
		return errors.New("usage: sleep <ticks>")
	}
	dt, err := strconv.ParseUint(args[0], 10, 32)
	if err != nil {
		return errors.New("sleep: invalid ticks")
	}
	if !s.timeCap.Valid() {
		return errors.New("sleep: no time capability")
	}
	return timeclient.Sleep(ctx, s.timeCap, uint32(dt))
}

func cmdVersion(ctx *kernel.Context, s *Service, _ []string, _ redirection) error {
	_ = s.printString(ctx, fmt.Sprintf("%s %s %s\n", buildinfo.Version, buildinfo.Commit, buildinfo.Date))
	return nil
}

func cmdUname(ctx *kernel.Context, s *Service, args []string, _ redirection) error {
	return s.uname(ctx, args)
}

func cmdFree(ctx *kernel.Context, s *Service, args []string, _ redirection) error {
	_ = ctx

	human := false
	if len(args) == 1 {
		if args[0] != "-h" {
			return errors.New("usage: free [-h]")
		}
		human = true
	} else if len(args) > 1 {
		return errors.New("usage: free [-h]")
	}

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	fmtVal := func(v uint64) string {
		if human {
			return fmtBytes(v)
		}
		return fmt.Sprintf("%d", v)
	}

	heapTotal := ms.HeapSys
	heapUsed := ms.HeapAlloc
	heapFree := uint64(0)
	if heapTotal >= heapUsed {
		heapFree = heapTotal - heapUsed
	}

	sysFree := uint64(0)
	if ms.Sys >= ms.Alloc {
		sysFree = ms.Sys - ms.Alloc
	}

	_ = s.printString(ctx, "           total       used       free\n")
	_ = s.printString(ctx, fmt.Sprintf("heap %11s %10s %10s\n", fmtVal(heapTotal), fmtVal(heapUsed), fmtVal(heapFree)))
	_ = s.printString(ctx, fmt.Sprintf("sys  %11s %10s %10s\n", fmtVal(ms.Sys), fmtVal(ms.Alloc), fmtVal(sysFree)))
	return nil
}

func cmdMux(ctx *kernel.Context, s *Service, args []string, _ redirection) error {
	if len(args) != 0 {
		return errors.New("usage: mux")
	}
	st, err := consolemuxclient.GetStatus(ctx, s.muxCap)
	if err != nil {
		return err
	}

	focus := "shell"
	if st.FocusApp {
		focus = "app"
	}
	app := appLabel(st.ActiveApp)
	if !st.HasApp {
		app += " (unavailable)"
	}

	_ = s.printString(ctx, fmt.Sprintf("active=%s focus=%s\n", app, focus))
	return nil
}

func cmdFocus(ctx *kernel.Context, s *Service, args []string, _ redirection) error {
	if len(args) == 0 {
		st, err := consolemuxclient.GetStatus(ctx, s.muxCap)
		if err != nil {
			return err
		}
		focus := "shell"
		if st.FocusApp {
			focus = "app"
		}
		_ = s.printString(ctx, fmt.Sprintf("focus=%s (toggle with Ctrl+G)\n", focus))
		return nil
	}
	if len(args) != 1 {
		return errors.New("usage: focus [app|shell|toggle]")
	}

	switch args[0] {
	case "app":
		return s.sendToMux(ctx, proto.MsgAppControl, proto.AppControlPayload(true))
	case "shell":
		return s.sendToMux(ctx, proto.MsgAppControl, proto.AppControlPayload(false))
	case "toggle":
		st, err := consolemuxclient.GetStatus(ctx, s.muxCap)
		if err != nil {
			return err
		}
		return s.sendToMux(ctx, proto.MsgAppControl, proto.AppControlPayload(!st.FocusApp))
	default:
		return errors.New("usage: focus [app|shell|toggle]")
	}
}

func appLabel(id proto.AppID) string {
	if name := appCommandName(id); name != "" {
		return fmt.Sprintf("%s(%d)", name, id)
	}
	return fmt.Sprintf("app(%d)", id)
}

func appCommandName(id proto.AppID) string {
	switch id {
	case proto.AppNone:
		return "none"
	case proto.AppRTDemo:
		return "rtdemo"
	case proto.AppVi:
		return "vi"
	case proto.AppRTVoxel:
		return "rtvoxel"
	case proto.AppImgView:
		return "imgview"
	case proto.AppMC:
		return "mc"
	case proto.AppHex:
		return "hex"
	case proto.AppVector:
		return "vector"
	case proto.AppSnake:
		return "snake"
	case proto.AppTetris:
		return "tetris"
	case proto.AppCalendar:
		return "cal"
	case proto.AppTodo:
		return "todo"
	case proto.AppArchive:
		return "arc"
	case proto.AppTEA:
		return "tea"
	case proto.AppBasic:
		return "basic"
	case proto.AppRFAnalyzer:
		return "rf"
	case proto.AppGPIOScope:
		return "gpio"
	case proto.AppFBTest:
		return "fbtest"
	case proto.AppSerialTerm:
		return "serial"
	case proto.AppUsers:
		return "users"
	case proto.AppQuarkDonut:
		return "donut"
	default:
		return ""
	}
}

func fmtBytes(v uint64) string {
	const (
		kib = 1024
		mib = 1024 * kib
		gib = 1024 * mib
	)

	switch {
	case v >= gib:
		return fmt.Sprintf("%.1fGiB", float64(v)/float64(gib))
	case v >= mib:
		return fmt.Sprintf("%.1fMiB", float64(v)/float64(mib))
	case v >= kib:
		return fmt.Sprintf("%.1fKiB", float64(v)/float64(kib))
	default:
		return fmt.Sprintf("%dB", v)
	}
}

func (s *Service) uname(ctx *kernel.Context, args []string) error {
	if len(args) == 0 {
		return s.printString(ctx, fmt.Sprintf("%s %s\n", runtime.GOOS, runtime.GOARCH))
	}
	if len(args) == 1 && args[0] == "-a" {
		sys := "SparkOS"
		node := "spark"
		rel := buildinfo.Short()
		ver := buildinfo.Commit
		mach := runtime.GOARCH
		return s.printString(ctx, fmt.Sprintf("%s %s %s %s %s\n", sys, node, rel, ver, mach))
	}
	return errors.New("usage: uname [-a]")
}
