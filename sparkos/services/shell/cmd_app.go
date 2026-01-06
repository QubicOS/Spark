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
		{Name: "hex", Usage: "hex <file>", Desc: "Hex viewer/editor (q/ESC to exit, w to save).", Run: cmdHex},
		{Name: "vector", Usage: "vector [expr]", Desc: "Math calculator with graphing (g graph, H help).", Run: cmdVector},
		{Name: "rtdemo", Usage: "rtdemo [on|off]", Desc: "Start raytracing demo (exit with q/ESC).", Run: cmdRTDemo},
		{Name: "rtvoxel", Usage: "rtvoxel [on|off]", Desc: "Start voxel world demo (exit with q/ESC).", Run: cmdRTVoxel},
		{Name: "imgview", Usage: "imgview <file>", Desc: "View an image (BMP/PNG/JPEG; q/ESC to exit).", Run: cmdImgView},
	} {
		if err := r.register(cmd); err != nil {
			return err
		}
	}
	return nil
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
