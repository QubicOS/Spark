package shell

import (
	"errors"
	"strings"

	"spark/sparkos/kernel"
	"spark/sparkos/proto"
	vitask "spark/sparkos/tasks/vi"
)

func registerAppCommands(r *registry) error {
	for _, cmd := range []command{
		{Name: "vi", Usage: "vi [file]", Desc: "Edit a file (SparkVi; build with -tags spark_vi).", Run: cmdVi},
		{Name: "mc", Usage: "mc [dir]", Desc: "Midnight Commander-like file manager (q/ESC to exit).", Run: cmdMC},
		{Name: "basic", Usage: "basic [file] | basic run <file>", Desc: "Tiny BASIC IDE (F1 code, F2 io, F3 vars).", Run: cmdBasic},
		{Name: "hex", Usage: "hex <file>", Desc: "Hex viewer/editor (q/ESC to exit, w to save).", Run: cmdHex},
		{Name: "vector", Usage: "vector [expr]", Desc: "Math calculator with graphing (g graph, H help).", Run: cmdVector},
		{Name: "snake", Usage: "snake", Desc: "Snake game (arrows move, p pause, r restart, q quit).", Run: cmdSnake},
		{Name: "tetris", Usage: "tetris", Desc: "Tetris (arrows move, z/x rotate, c drop, p pause, r restart, q quit).", Run: cmdTetris},
		{Name: "cal", Aliases: []string{"calendar"}, Usage: "cal [YYYY-MM[-DD]]", Desc: "Calendar (arrows move, Enter day view, a add, d delete, n/b month, q quit).", Run: cmdCalendar},
		{Name: "todo", Usage: "todo [all|open|done|search]", Desc: "TODO list (a add, e edit, d delete, p prio, f filter, / search).", Run: cmdTodo},
		{Name: "arc", Aliases: []string{"archive"}, Usage: "arc <file>", Desc: "Archive manager (tar/zip; x extract, c create).", Run: cmdArchive},
		{Name: "tea", Usage: "tea [file|dir]", Desc: "TEA audio player (Enter play, Space pause, s stop, +/- volume).", Run: cmdTEA},
		{Name: "rtdemo", Usage: "rtdemo [on|off]", Desc: "Start raytracing demo (exit with q/ESC).", Run: cmdRTDemo},
		{Name: "rtvoxel", Usage: "rtvoxel [on|off]", Desc: "Start voxel world demo (exit with q/ESC).", Run: cmdRTVoxel},
		{Name: "imgview", Usage: "imgview <file>", Desc: "View an image (BMP/PNG/JPEG; q/ESC to exit).", Run: cmdImgView},
		{Name: "rf", Usage: "rf", Desc: "2.4 GHz RF Analyzer (nRF24 scan + waterfall + sniffer).", Run: cmdRFAnalyzer},
	} {
		if err := r.register(cmd); err != nil {
			return err
		}
	}
	return nil
}

func cmdRFAnalyzer(ctx *kernel.Context, s *Service, args []string, _ redirection) error {
	if len(args) != 0 {
		return errors.New("usage: rf")
	}
	if err := s.sendToMux(ctx, proto.MsgAppSelect, proto.AppSelectPayload(proto.AppRFAnalyzer, "")); err != nil {
		return err
	}
	return s.sendToMux(ctx, proto.MsgAppControl, proto.AppControlPayload(true))
}

func cmdVi(ctx *kernel.Context, s *Service, args []string, _ redirection) error {
	if !vitask.Enabled {
		return errors.New("not enabled in this build (build with -tags spark_vi)")
	}

	var target string
	if len(args) == 1 {
		target = s.absPath(args[0])
	} else if len(args) > 1 {
		return errors.New("usage: vi [file]")
	}

	if err := s.sendToMux(ctx, proto.MsgAppSelect, proto.AppSelectPayload(proto.AppVi, target)); err != nil {
		return err
	}
	return s.sendToMux(ctx, proto.MsgAppControl, proto.AppControlPayload(true))
}

func cmdMC(ctx *kernel.Context, s *Service, args []string, _ redirection) error {
	var target string
	if len(args) == 1 {
		target = s.absPath(args[0])
	} else if len(args) > 1 {
		return errors.New("usage: mc [dir]")
	} else {
		target = s.cwd
	}

	if err := s.sendToMux(ctx, proto.MsgAppSelect, proto.AppSelectPayload(proto.AppMC, target)); err != nil {
		return err
	}
	return s.sendToMux(ctx, proto.MsgAppControl, proto.AppControlPayload(true))
}

func cmdBasic(ctx *kernel.Context, s *Service, args []string, _ redirection) error {
	var arg string
	switch len(args) {
	case 0:
		arg = ""
	case 1:
		arg = s.absPath(args[0])
	case 2:
		if args[0] != "run" {
			return errors.New("usage: basic [file] | basic run <file>")
		}
		arg = "run:" + s.absPath(args[1])
	default:
		return errors.New("usage: basic [file] | basic run <file>")
	}
	if err := s.sendToMux(ctx, proto.MsgAppSelect, proto.AppSelectPayload(proto.AppBasic, arg)); err != nil {
		return err
	}
	return s.sendToMux(ctx, proto.MsgAppControl, proto.AppControlPayload(true))
}

func cmdHex(ctx *kernel.Context, s *Service, args []string, _ redirection) error {
	if len(args) != 1 {
		return errors.New("usage: hex <file>")
	}
	target := s.absPath(args[0])

	if err := s.sendToMux(ctx, proto.MsgAppSelect, proto.AppSelectPayload(proto.AppHex, target)); err != nil {
		return err
	}
	return s.sendToMux(ctx, proto.MsgAppControl, proto.AppControlPayload(true))
}

func cmdVector(ctx *kernel.Context, s *Service, args []string, _ redirection) error {
	var expr string
	if len(args) > 0 {
		expr = strings.Join(args, " ")
	}

	if err := s.sendToMux(ctx, proto.MsgAppSelect, proto.AppSelectPayload(proto.AppVector, expr)); err != nil {
		return err
	}
	return s.sendToMux(ctx, proto.MsgAppControl, proto.AppControlPayload(true))
}

func cmdSnake(ctx *kernel.Context, s *Service, args []string, _ redirection) error {
	if len(args) != 0 {
		return errors.New("usage: snake")
	}
	if err := s.sendToMux(ctx, proto.MsgAppSelect, proto.AppSelectPayload(proto.AppSnake, "")); err != nil {
		return err
	}
	return s.sendToMux(ctx, proto.MsgAppControl, proto.AppControlPayload(true))
}

func cmdTetris(ctx *kernel.Context, s *Service, args []string, _ redirection) error {
	if len(args) != 0 {
		return errors.New("usage: tetris")
	}
	if err := s.sendToMux(ctx, proto.MsgAppSelect, proto.AppSelectPayload(proto.AppTetris, "")); err != nil {
		return err
	}
	return s.sendToMux(ctx, proto.MsgAppControl, proto.AppControlPayload(true))
}

func cmdCalendar(ctx *kernel.Context, s *Service, args []string, _ redirection) error {
	var arg string
	if len(args) == 1 {
		arg = args[0]
	} else if len(args) > 1 {
		return errors.New("usage: cal [YYYY-MM[-DD]]")
	}

	if err := s.sendToMux(ctx, proto.MsgAppSelect, proto.AppSelectPayload(proto.AppCalendar, arg)); err != nil {
		return err
	}
	return s.sendToMux(ctx, proto.MsgAppControl, proto.AppControlPayload(true))
}

func cmdTodo(ctx *kernel.Context, s *Service, args []string, _ redirection) error {
	var arg string
	if len(args) == 1 {
		arg = args[0]
	} else if len(args) > 1 {
		return errors.New("usage: todo [all|open|done|search]")
	}

	if err := s.sendToMux(ctx, proto.MsgAppSelect, proto.AppSelectPayload(proto.AppTodo, arg)); err != nil {
		return err
	}
	return s.sendToMux(ctx, proto.MsgAppControl, proto.AppControlPayload(true))
}

func cmdArchive(ctx *kernel.Context, s *Service, args []string, _ redirection) error {
	if len(args) != 1 {
		return errors.New("usage: arc <file>")
	}
	target := s.absPath(args[0])

	if err := s.sendToMux(ctx, proto.MsgAppSelect, proto.AppSelectPayload(proto.AppArchive, target)); err != nil {
		return err
	}
	return s.sendToMux(ctx, proto.MsgAppControl, proto.AppControlPayload(true))
}

func cmdTEA(ctx *kernel.Context, s *Service, args []string, _ redirection) error {
	var target string
	if len(args) == 1 {
		target = s.absPath(args[0])
	} else if len(args) > 1 {
		return errors.New("usage: tea [file|dir]")
	}

	if err := s.sendToMux(ctx, proto.MsgAppSelect, proto.AppSelectPayload(proto.AppTEA, target)); err != nil {
		return err
	}
	return s.sendToMux(ctx, proto.MsgAppControl, proto.AppControlPayload(true))
}

func cmdRTDemo(ctx *kernel.Context, s *Service, args []string, _ redirection) error {
	active := true
	if len(args) == 1 {
		switch args[0] {
		case "on":
			active = true
		case "off":
			active = false
		default:
			return errors.New("usage: rtdemo [on|off]")
		}
	} else if len(args) > 1 {
		return errors.New("usage: rtdemo [on|off]")
	}

	if active {
		if err := s.sendToMux(ctx, proto.MsgAppSelect, proto.AppSelectPayload(proto.AppRTDemo, "")); err != nil {
			return err
		}
	}
	return s.sendToMux(ctx, proto.MsgAppControl, proto.AppControlPayload(active))
}

func cmdRTVoxel(ctx *kernel.Context, s *Service, args []string, _ redirection) error {
	active := true
	if len(args) == 1 {
		switch args[0] {
		case "on":
			active = true
		case "off":
			active = false
		default:
			return errors.New("usage: rtvoxel [on|off]")
		}
	} else if len(args) > 1 {
		return errors.New("usage: rtvoxel [on|off]")
	}

	if active {
		if err := s.sendToMux(ctx, proto.MsgAppSelect, proto.AppSelectPayload(proto.AppRTVoxel, "")); err != nil {
			return err
		}
	}
	return s.sendToMux(ctx, proto.MsgAppControl, proto.AppControlPayload(active))
}

func cmdImgView(ctx *kernel.Context, s *Service, args []string, _ redirection) error {
	if len(args) != 1 {
		return errors.New("usage: imgview <file>")
	}
	target := s.absPath(args[0])

	if err := s.sendToMux(ctx, proto.MsgAppSelect, proto.AppSelectPayload(proto.AppImgView, target)); err != nil {
		return err
	}
	return s.sendToMux(ctx, proto.MsgAppControl, proto.AppControlPayload(true))
}
