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

	line   []rune
	cursor int

	utf8buf []byte

	history []string
	histPos int
	scratch []rune
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

func (s *Service) moveLeft(ctx *kernel.Context) {
	if s.cursor <= 0 {
		return
	}
	_ = s.writeString(ctx, "\x1b[D")
	s.cursor--
}

func (s *Service) moveRight(ctx *kernel.Context) {
	if s.cursor >= len(s.line) {
		return
	}
	_ = s.writeString(ctx, "\x1b[C")
	s.cursor++
}

func (s *Service) home(ctx *kernel.Context) {
	for s.cursor > 0 {
		s.moveLeft(ctx)
	}
}

func (s *Service) end(ctx *kernel.Context) {
	for s.cursor < len(s.line) {
		s.moveRight(ctx)
	}
}

func (s *Service) insertRune(ctx *kernel.Context, r rune) {
	if s.cursor == len(s.line) {
		s.line = append(s.line, r)
		s.cursor++
		_ = s.writeString(ctx, string(r))
		return
	}
	s.line = append(s.line[:s.cursor], append([]rune{r}, s.line[s.cursor:]...)...)
	_ = s.writeString(ctx, string(r))
	s.cursor++
	_ = s.redrawFromCursor(ctx)
}

func (s *Service) deleteForward(ctx *kernel.Context) {
	if s.cursor >= len(s.line) {
		return
	}
	s.line = append(s.line[:s.cursor], s.line[s.cursor+1:]...)
	_ = s.redrawFromCursor(ctx)
}

func (s *Service) redrawFromCursor(ctx *kernel.Context) error {
	tail := s.line[s.cursor:]
	// Print tail and clear one extra cell to erase leftovers, then restore cursor position.
	if err := s.writeString(ctx, string(tail)); err != nil {
		return err
	}
	if err := s.writeString(ctx, " "); err != nil {
		return err
	}
	for i := 0; i < len(tail)+1; i++ {
		if err := s.writeString(ctx, "\x1b[D"); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) backspace(ctx *kernel.Context) {
	if len(s.line) == 0 || s.cursor == 0 {
		return
	}
	s.cursor--
	s.line = append(s.line[:s.cursor], s.line[s.cursor+1:]...)

	_ = s.writeString(ctx, "\x1b[D")
	_ = s.redrawFromCursor(ctx)
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
	s.cursor = 0
	return s.writeString(ctx, "\x1b[38;5;46m>\x1b[0m ")
}

func (s *Service) histUp(ctx *kernel.Context) {
	if len(s.history) == 0 {
		return
	}
	if s.histPos == len(s.history) {
		s.scratch = append(s.scratch[:0], s.line...)
	}
	if s.histPos <= 0 {
		return
	}
	s.histPos--
	s.replaceLine(ctx, []rune(s.history[s.histPos]))
}

func (s *Service) histDown(ctx *kernel.Context) {
	if len(s.history) == 0 {
		return
	}
	if s.histPos >= len(s.history) {
		return
	}
	s.histPos++
	if s.histPos == len(s.history) {
		s.replaceLine(ctx, s.scratch)
		return
	}
	s.replaceLine(ctx, []rune(s.history[s.histPos]))
}

func (s *Service) replaceLine(ctx *kernel.Context, r []rune) {
	for s.cursor > 0 {
		_ = s.writeString(ctx, "\x1b[D")
		s.cursor--
	}
	for range s.line {
		_ = s.writeString(ctx, " ")
	}
	for range s.line {
		_ = s.writeString(ctx, "\x1b[D")
	}
	s.line = s.line[:0]
	s.line = append(s.line, r...)
	s.cursor = len(s.line)
	_ = s.writeString(ctx, string(s.line))
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

type escAction uint8

const (
	escNone escAction = iota
	escUp
	escDown
	escRight
	escLeft
	escDelete
	escHome
	escEnd
)

func parseEscape(b []byte) (consumed int, action escAction, ok bool) {
	if len(b) < 2 || b[0] != 0x1b {
		return 0, escNone, true
	}
	if b[1] != '[' {
		if len(b) < 2 {
			return 0, escNone, false
		}
		return 2, escNone, true
	}
	if len(b) < 3 {
		return 0, escNone, false
	}
	switch b[2] {
	case 'A':
		return 3, escUp, true
	case 'B':
		return 3, escDown, true
	case 'C':
		return 3, escRight, true
	case 'D':
		return 3, escLeft, true
	case 'H':
		return 3, escHome, true
	case 'F':
		return 3, escEnd, true
	case '3':
		// CSI 3 ~ : Delete
		if len(b) < 4 {
			return 0, escNone, false
		}
		if b[3] == '~' {
			return 4, escDelete, true
		}
		return consumeEscape(b), escNone, true
	case '1':
		// CSI 1 ~ : Home
		if len(b) < 4 {
			return 0, escNone, false
		}
		if b[3] == '~' {
			return 4, escHome, true
		}
		return consumeEscape(b), escNone, true
	case '4':
		// CSI 4 ~ : End
		if len(b) < 4 {
			return 0, escNone, false
		}
		if b[3] == '~' {
			return 4, escEnd, true
		}
		return consumeEscape(b), escNone, true
	default:
		return consumeEscape(b), escNone, true
	}
}

func consumeEscape(b []byte) int {
	if len(b) < 2 || b[0] != 0x1b {
		return 0
	}
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
