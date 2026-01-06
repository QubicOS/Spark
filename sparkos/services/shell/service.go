package shell

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

type Service struct {
	inCap   kernel.Capability
	termCap kernel.Capability

	line []rune

	utf8buf []byte
}

func New(inCap kernel.Capability, termCap kernel.Capability) *Service {
	return &Service{inCap: inCap, termCap: termCap}
}

func (s *Service) Run(ctx *kernel.Context) {
	ch, ok := ctx.RecvChan(s.inCap)
	if !ok {
		return
	}

	_ = s.writeString(ctx, "\x1b[0m")
	_ = s.writeString(ctx, "\x1b[38;5;39mSparkOS shell\x1b[0m\n")
	_ = s.writeString(ctx, "Type `help`.\n\n")
	_ = s.prompt(ctx)

	for msg := range ch {
		if proto.Kind(msg.Kind) != proto.MsgTermInput {
			continue
		}
		s.handleInput(ctx, msg.Data[:msg.Len])
	}
}

func (s *Service) handleInput(ctx *kernel.Context, b []byte) {
	s.utf8buf = append(s.utf8buf, b...)
	b = s.utf8buf

	for len(b) > 0 {
		if b[0] == 0x1b {
			// Best-effort: skip VT100 escape sequences (e.g. arrows).
			consumed := consumeEscape(b)
			if consumed == 0 {
				b = b[1:]
			} else {
				b = b[consumed:]
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
			s.line = append(s.line, r)
			_ = s.writeString(ctx, string(r))
		}
	}
	s.utf8buf = s.utf8buf[:0]
}

func (s *Service) backspace(ctx *kernel.Context) {
	if len(s.line) == 0 {
		return
	}
	s.line = s.line[:len(s.line)-1]
	// Move cursor left, overwrite, move left.
	_ = s.writeString(ctx, "\x1b[D \x1b[D")
}

func (s *Service) submit(ctx *kernel.Context) {
	_ = s.writeString(ctx, "\n")

	line := strings.TrimSpace(string(s.line))
	s.line = s.line[:0]
	if line == "" {
		_ = s.prompt(ctx)
		return
	}

	args := strings.Fields(line)
	cmd := args[0]
	args = args[1:]

	switch cmd {
	case "help":
		_ = s.writeString(ctx, "commands: help clear echo ticks\n")
	case "clear":
		_ = s.sendToTerm(ctx, proto.MsgTermClear, nil)
	case "echo":
		_ = s.writeString(ctx, strings.Join(args, " ")+"\n")
	case "ticks":
		_ = s.writeString(ctx, fmt.Sprintf("%d\n", ctx.NowTick()))
	default:
		_ = s.writeString(ctx, "unknown command: "+cmd+"\n")
	}

	_ = s.prompt(ctx)
}

func (s *Service) prompt(ctx *kernel.Context) error {
	return s.writeString(ctx, "\x1b[38;5;46m>\x1b[0m ")
}

func (s *Service) writeString(ctx *kernel.Context, s2 string) error {
	return s.writeBytes(ctx, []byte(s2))
}

func (s *Service) writeBytes(ctx *kernel.Context, b []byte) error {
	for len(b) > 0 {
		chunk := b
		if len(chunk) > kernel.MaxMessageBytes {
			chunk = chunk[:kernel.MaxMessageBytes]
		}
		if err := s.sendToTerm(ctx, proto.MsgTermWrite, chunk); err != nil {
			return err
		}
		b = b[len(chunk):]
	}
	return nil
}

func (s *Service) sendToTerm(ctx *kernel.Context, kind proto.Kind, payload []byte) error {
	for {
		res := ctx.SendToCapResult(s.termCap, uint16(kind), payload, kernel.Capability{})
		switch res {
		case kernel.SendOK:
			return nil
		case kernel.SendErrQueueFull:
			ctx.BlockOnTick()
		default:
			return fmt.Errorf("shell term send: %s", res)
		}
	}
}

func consumeEscape(b []byte) int {
	if len(b) < 2 || b[0] != 0x1b {
		return 0
	}
	// CSI: ESC [ ... final
	if b[1] == '[' {
		for i := 2; i < len(b); i++ {
			if b[i] >= 0x40 && b[i] <= 0x7e {
				return i + 1
			}
		}
		return len(b)
	}
	return 2
}
