package shell

import (
	"fmt"
	"runtime"
	"strings"
	"unicode/utf8"

	"spark/internal/buildinfo"
	logclient "spark/sparkos/client/logger"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

type Service struct {
	inCap   kernel.Capability
	termCap kernel.Capability
	logCap  kernel.Capability

	line   []rune
	cursor int

	utf8buf []byte

	history []string
	histPos int
	scratch []rune
}

func New(inCap kernel.Capability, termCap kernel.Capability, logCap kernel.Capability) *Service {
	return &Service{inCap: inCap, termCap: termCap, logCap: logCap}
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
	s.line = append(s.line, 0)
	copy(s.line[s.cursor+1:], s.line[s.cursor:])
	s.line[s.cursor] = r
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
	if err := s.writeString(ctx, string(tail)); err != nil {
		return err
	}
	if err := s.writeString(ctx, "\x1b[K"); err != nil {
		return err
	}
	for range tail {
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

func (s *Service) killLeft(ctx *kernel.Context) {
	if s.cursor <= 0 {
		return
	}
	s.line = append([]rune{}, s.line[s.cursor:]...)
	s.cursor = 0
	_ = s.redrawLine(ctx)
}

func (s *Service) deletePrevWord(ctx *kernel.Context) {
	if s.cursor <= 0 {
		return
	}
	i := s.cursor
	for i > 0 && s.line[i-1] == ' ' {
		i--
	}
	for i > 0 && s.line[i-1] != ' ' {
		i--
	}
	if i == s.cursor {
		return
	}
	s.line = append(s.line[:i], s.line[s.cursor:]...)
	s.cursor = i
	_ = s.redrawLine(ctx)
}

func (s *Service) cancelLine(ctx *kernel.Context) {
	if len(s.line) == 0 {
		_ = s.writeString(ctx, "\n")
		_ = s.prompt(ctx)
		return
	}
	_ = s.writeString(ctx, "\n")
	s.line = s.line[:0]
	s.cursor = 0
	s.utf8buf = s.utf8buf[:0]
	_ = s.prompt(ctx)
}

func (s *Service) tab(ctx *kernel.Context) {
	if s.cursor != len(s.line) {
		return
	}
	if s.cursor == 0 {
		return
	}
	if strings.IndexByte(string(s.line[:s.cursor]), ' ') >= 0 {
		// TODO: arg completion (later via IPC).
		return
	}

	prefix := string(s.line[:s.cursor])
	matches := s.commandMatches(prefix)
	if len(matches) == 0 {
		return
	}

	common := matches[0]
	for _, m := range matches[1:] {
		common = commonPrefix(common, m)
	}
	if len(common) > len(prefix) {
		s.insertString(ctx, common[len(prefix):])
		prefix = common
	}

	if len(matches) == 1 {
		s.insertString(ctx, " ")
		return
	}

	_ = s.writeString(ctx, "\n")
	for _, m := range matches {
		_ = s.writeString(ctx, m+"\n")
	}
	_ = s.redrawLine(ctx)
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
		for _, c := range builtinCommandHelp {
			_ = s.writeString(ctx, fmt.Sprintf("%-10s %s\n", c.Name, c.Desc))
		}
	case "clear":
		_ = s.sendToTerm(ctx, proto.MsgTermClear, nil)
	case "echo":
		_ = s.writeString(ctx, strings.Join(args, " ")+"\n")
	case "ticks":
		_ = s.writeString(ctx, fmt.Sprintf("%d\n", ctx.NowTick()))
	case "version":
		_ = s.writeString(ctx, fmt.Sprintf("%s %s %s\n", buildinfo.Version, buildinfo.Commit, buildinfo.Date))
	case "uname":
		_ = s.writeString(ctx, fmt.Sprintf("%s %s\n", runtime.GOOS, runtime.GOARCH))
	case "panic":
		panic("shell panic")
	case "log":
		if len(args) == 0 {
			_ = s.writeString(ctx, "usage: log <line>\n")
			break
		}
		logLine := strings.Join(args, " ")
		res := logclient.Log(ctx, s.logCap, logLine)
		if res != kernel.SendOK {
			_ = s.writeString(ctx, "logger: "+res.String()+"\n")
		}
	default:
		_ = s.writeString(ctx, "unknown command: "+cmd+"\n")
	}

	_ = s.prompt(ctx)
}

func (s *Service) prompt(ctx *kernel.Context) error {
	s.cursor = 0
	return s.writeString(ctx, promptANSI)
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
	s.line = s.line[:0]
	s.line = append(s.line, r...)
	s.cursor = len(s.line)
	_ = s.redrawLine(ctx)
}

func (s *Service) redrawLine(ctx *kernel.Context) error {
	if err := s.writeString(ctx, "\x1b[1G\x1b[2K"); err != nil {
		return err
	}
	if err := s.writeString(ctx, promptANSI); err != nil {
		return err
	}
	if err := s.writeString(ctx, string(s.line)); err != nil {
		return err
	}
	if err := s.writeString(ctx, "\x1b[K"); err != nil {
		return err
	}
	col := promptCols + 1 + s.cursor
	return s.writeString(ctx, fmt.Sprintf("\x1b[%dG", col))
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

func (s *Service) insertString(ctx *kernel.Context, tail string) {
	rs := []rune(tail)
	if len(rs) == 0 {
		return
	}
	if len(s.line)+len(rs) > 256 {
		rs = rs[:maxInt(0, 256-len(s.line))]
	}
	if len(rs) == 0 {
		return
	}
	s.line = append(s.line, rs...)
	s.cursor = len(s.line)
	_ = s.writeString(ctx, string(rs))
}

func (s *Service) commandMatches(prefix string) []string {
	var out []string
	for _, cmd := range builtinCommands {
		if strings.HasPrefix(cmd, prefix) {
			out = append(out, cmd)
		}
	}
	return out
}

func commonPrefix(a, b string) string {
	n := minInt(len(a), len(b))
	i := 0
	for i < n && a[i] == b[i] {
		i++
	}
	return a[:i]
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

var builtinCommands = []string{
	"clear",
	"echo",
	"help",
	"log",
	"panic",
	"ticks",
	"uname",
	"version",
}

type commandHelp struct {
	Name string
	Desc string
}

var builtinCommandHelp = []commandHelp{
	{Name: "help", Desc: "Show available commands."},
	{Name: "clear", Desc: "Clear the terminal."},
	{Name: "echo", Desc: "Print arguments."},
	{Name: "ticks", Desc: "Show current kernel tick counter."},
	{Name: "version", Desc: "Show build version."},
	{Name: "uname", Desc: "Show runtime OS/arch."},
	{Name: "panic", Desc: "Panic the shell task (test)."},
	{Name: "log", Desc: "Send a log line to logger service."},
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
