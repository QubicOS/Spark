package shell

import (
	"strings"
	"unicode/utf8"

	vfsclient "spark/sparkos/client/vfs"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

type Service struct {
	inCap   kernel.Capability
	termCap kernel.Capability
	logCap  kernel.Capability
	vfsCap  kernel.Capability
	timeCap kernel.Capability
	muxCap  kernel.Capability

	vfs *vfsclient.Client
	reg *registry

	line   []rune
	cursor int

	utf8buf []byte

	history []string
	histPos int
	scratch []rune

	scrollback []string

	cwd string

	hint  string
	ghost string
	cands []string
	best  string
}

func New(inCap kernel.Capability, termCap kernel.Capability, logCap kernel.Capability, vfsCap kernel.Capability, timeCap kernel.Capability, muxCap kernel.Capability) *Service {
	return &Service{inCap: inCap, termCap: termCap, logCap: logCap, vfsCap: vfsCap, timeCap: timeCap, muxCap: muxCap}
}

const (
	promptANSI = "\x1b[38;5;46m>\x1b[0m "
	promptCols = 2
)

func (s *Service) Run(ctx *kernel.Context) {
	ch, ok := ctx.RecvChan(s.inCap)
	if !ok {
		return
	}

	if s.cwd == "" {
		s.cwd = "/"
	}
	if s.reg == nil {
		if err := s.initRegistry(); err != nil {
			_ = s.printString(ctx, "shell: init: "+err.Error()+"\n")
			return
		}
	}

	_ = s.printString(ctx, "\x1b[0m")
	_ = s.printString(ctx, "\x1b[38;5;39mSparkOS shell\x1b[0m\n")
	_ = s.printString(ctx, "Type `help`.\n\n")
	_ = s.prompt(ctx)

	for msg := range ch {
		switch proto.Kind(msg.Kind) {
		case proto.MsgTermInput:
			s.handleInput(ctx, msg.Data[:msg.Len])
		case proto.MsgAppControl:
			active, ok := proto.DecodeAppControlPayload(msg.Data[:msg.Len])
			if !ok {
				continue
			}
			s.handleFocus(ctx, active)
		}
	}
}

func (s *Service) handleFocus(ctx *kernel.Context, active bool) {
	if !active {
		return
	}
	s.utf8buf = s.utf8buf[:0]
	_ = s.sendToTerm(ctx, proto.MsgTermClear, nil)
	_ = s.writeString(ctx, "\x1b[0m")
	_ = s.redrawLine(ctx)
	_ = s.sendToTerm(ctx, proto.MsgTermRefresh, nil)
}

func (s *Service) handleInput(ctx *kernel.Context, b []byte) {
	s.utf8buf = append(s.utf8buf, b...)
	b = s.utf8buf

	for len(b) > 0 {
		if b[0] == 0x1b {
			consumed, act, ok := parseEscape(b)
			if !ok {
				s.utf8buf = b
				return
			}
			b = b[consumed:]

			switch act {
			case escUp:
				s.histUp(ctx)
			case escDown:
				s.histDown(ctx)
			case escLeft:
				s.moveLeft(ctx)
			case escRight:
				s.moveRight(ctx)
			case escDelete:
				s.deleteForward(ctx)
			case escHome:
				s.home(ctx)
			case escEnd:
				s.end(ctx)
			}
			continue
		}

		switch b[0] {
		case '\r':
			b = b[1:]
		case '\n':
			b = b[1:]
			s.submit(ctx)
		case 0x7f, 0x08:
			b = b[1:]
			s.backspace(ctx)
		case '\t':
			b = b[1:]
			s.tab(ctx)
		case 0x01:
			// Ctrl+A.
			b = b[1:]
			s.home(ctx)
		case 0x05:
			// Ctrl+E.
			b = b[1:]
			s.end(ctx)
		case 0x15:
			// Ctrl+U.
			b = b[1:]
			s.killLeft(ctx)
		case 0x17:
			// Ctrl+W.
			b = b[1:]
			s.deletePrevWord(ctx)
		case 0x03:
			// Ctrl+C.
			b = b[1:]
			s.cancelLine(ctx)
		default:
			if !utf8.FullRune(b) {
				s.utf8buf = b
				return
			}
			r, sz := utf8.DecodeRune(b)
			if r == utf8.RuneError && sz == 1 {
				b = b[1:]
				continue
			}
			b = b[sz:]

			if r < 0x20 {
				continue
			}
			if len(s.line) >= 256 {
				continue
			}
			s.insertRune(ctx, r)
		}
	}
	s.utf8buf = s.utf8buf[:0]
}

func (s *Service) submit(ctx *kernel.Context) {
	_ = s.writeString(ctx, "\n")

	line := strings.TrimSpace(string(s.line))
	s.line = s.line[:0]
	s.cursor = 0
	s.scratch = s.scratch[:0]
	s.histPos = len(s.history)
	if line == "" {
		_ = s.prompt(ctx)
		return
	}

	if len(s.history) == 0 || s.history[len(s.history)-1] != line {
		s.history = append(s.history, line)
	}
	s.histPos = len(s.history)

	args, redirect, ok := parseArgs(line)
	if !ok || len(args) == 0 {
		_ = s.prompt(ctx)
		return
	}
	name := args[0]
	args = args[1:]

	cmd, ok := s.reg.resolve(name)
	if !ok {
		_ = s.printString(ctx, "unknown command: "+name+"\n")
		_ = s.prompt(ctx)
		return
	}
	if err := cmd.Run(ctx, s, args, redirect); err != nil {
		_ = s.printString(ctx, cmd.Name+": "+err.Error()+"\n")
	}
	if redirect.Path != "" && cmd.Name != "echo" && cmd.Name != "cat" {
		_ = s.printString(ctx, "redirect: not supported for "+cmd.Name+"\n")
	}

	_ = s.prompt(ctx)
}
