package shell

import (
	"errors"
	"fmt"
	"runtime"
	"strconv"

	"spark/internal/buildinfo"
	timeclient "spark/sparkos/client/time"
	"spark/sparkos/kernel"
)

func registerSysCommands(r *registry) error {
	for _, cmd := range []command{
		{Name: "ticks", Usage: "ticks", Desc: "Show current kernel tick counter.", Run: cmdTicks},
		{Name: "uptime", Usage: "uptime", Desc: "Show uptime (ticks).", Run: cmdUptime},
		{Name: "sleep", Usage: "sleep <ticks>", Desc: "Sleep for dt ticks via time service.", Run: cmdSleep},
		{Name: "version", Usage: "version", Desc: "Show build version.", Run: cmdVersion},
		{Name: "uname", Usage: "uname [-a]", Desc: "Show system information.", Run: cmdUname},
		{Name: "free", Usage: "free [-h]", Desc: "Show memory usage.", Run: cmdFree},
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
