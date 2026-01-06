package shell

import (
	"errors"
	"fmt"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"spark/internal/buildinfo"
	logclient "spark/sparkos/client/logger"
	vfsclient "spark/sparkos/client/vfs"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

type Service struct {
	inCap   kernel.Capability
	termCap kernel.Capability
	logCap  kernel.Capability
	vfsCap  kernel.Capability
	muxCap  kernel.Capability

	vfs *vfsclient.Client

	line   []rune
	cursor int

	utf8buf []byte

	history []string
	histPos int
	scratch []rune

	scrollback []string

	cwd string
}

func New(inCap kernel.Capability, termCap kernel.Capability, logCap kernel.Capability, vfsCap kernel.Capability, muxCap kernel.Capability) *Service {
	return &Service{inCap: inCap, termCap: termCap, logCap: logCap, vfsCap: vfsCap, muxCap: muxCap}
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

	_ = s.printString(ctx, "\x1b[0m")
	_ = s.printString(ctx, "\x1b[38;5;39mSparkOS shell\x1b[0m\n")
	_ = s.printString(ctx, "Type `help`.\n\n")
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

	args, redirect, ok := parseArgs(line)
	if !ok || len(args) == 0 {
		_ = s.prompt(ctx)
		return
	}
	cmd := args[0]
	args = args[1:]

	switch cmd {
	case "help":
		for _, c := range builtinCommandHelp {
			_ = s.printString(ctx, fmt.Sprintf("%-10s %s\n", c.Name, c.Desc))
		}
	case "clear":
		_ = s.sendToTerm(ctx, proto.MsgTermClear, nil)
	case "echo":
		if err := s.echo(ctx, args, redirect); err != nil {
			_ = s.printString(ctx, "echo: "+err.Error()+"\n")
		}
	case "ticks":
		_ = s.printString(ctx, fmt.Sprintf("%d\n", ctx.NowTick()))
	case "uptime":
		_ = s.printString(ctx, fmt.Sprintf("up %d ticks\n", ctx.NowTick()))
	case "version":
		_ = s.printString(ctx, fmt.Sprintf("%s %s %s\n", buildinfo.Version, buildinfo.Commit, buildinfo.Date))
	case "uname":
		if err := s.uname(ctx, args); err != nil {
			_ = s.printString(ctx, "uname: "+err.Error()+"\n")
		}
	case "panic":
		panic("shell panic")
	case "log":
		if len(args) == 0 {
			_ = s.printString(ctx, "usage: log <line>\n")
			break
		}
		logLine := strings.Join(args, " ")
		res := logclient.Log(ctx, s.logCap, logLine)
		if res != kernel.SendOK {
			_ = s.printString(ctx, "logger: "+res.String()+"\n")
		}
	case "scrollback":
		n := 50
		if len(args) >= 1 {
			if parsed, err := strconv.Atoi(args[0]); err == nil && parsed > 0 {
				n = parsed
			}
		}
		start := len(s.scrollback) - n
		if start < 0 {
			start = 0
		}
		if start >= len(s.scrollback) {
			_ = s.writeString(ctx, "(empty)\n")
			break
		}
		for _, ln := range s.scrollback[start:] {
			_ = s.writeString(ctx, ln+"\n")
		}
	case "ls":
		if err := s.ls(ctx, args); err != nil {
			_ = s.printString(ctx, "ls: "+err.Error()+"\n")
		}
	case "pwd":
		_ = s.printString(ctx, s.cwd+"\n")
	case "cd":
		if err := s.cd(ctx, args); err != nil {
			_ = s.printString(ctx, "cd: "+err.Error()+"\n")
		}
	case "mkdir":
		if err := s.mkdir(ctx, args); err != nil {
			_ = s.printString(ctx, "mkdir: "+err.Error()+"\n")
		}
	case "touch":
		if err := s.touch(ctx, args); err != nil {
			_ = s.printString(ctx, "touch: "+err.Error()+"\n")
		}
	case "cp":
		if err := s.cp(ctx, args); err != nil {
			_ = s.printString(ctx, "cp: "+err.Error()+"\n")
		}
	case "stat":
		if err := s.stat(ctx, args); err != nil {
			_ = s.printString(ctx, "stat: "+err.Error()+"\n")
		}
	case "cat":
		if err := s.cat(ctx, args, redirect); err != nil {
			_ = s.printString(ctx, "cat: "+err.Error()+"\n")
		}
	case "put":
		if err := s.put(ctx, args); err != nil {
			_ = s.printString(ctx, "put: "+err.Error()+"\n")
		}
	case "rtdemo":
		active := true
		if len(args) == 1 {
			switch args[0] {
			case "on":
				active = true
			case "off":
				active = false
			default:
				_ = s.printString(ctx, "usage: rtdemo [on|off]\n")
				break
			}
		} else if len(args) > 1 {
			_ = s.printString(ctx, "usage: rtdemo [on|off]\n")
			break
		}
		if err := s.sendToMux(ctx, proto.MsgAppControl, proto.AppControlPayload(active)); err != nil {
			_ = s.printString(ctx, "rtdemo: "+err.Error()+"\n")
		}
	default:
		_ = s.printString(ctx, "unknown command: "+cmd+"\n")
	}

	if redirect.Path != "" && cmd != "echo" && cmd != "cat" {
		_ = s.printString(ctx, "redirect: not supported for "+cmd+"\n")
	}

	_ = s.prompt(ctx)
}

func (s *Service) cd(ctx *kernel.Context, args []string) error {
	target := "/"
	if len(args) == 1 {
		target = args[0]
	} else if len(args) > 1 {
		return errors.New("usage: cd [dir]")
	}
	target = s.absPath(target)

	typ, _, err := s.vfsClient().Stat(ctx, target)
	if err != nil {
		return err
	}
	if typ != proto.VFSEntryDir {
		return errors.New("not a directory")
	}
	s.cwd = target
	return nil
}

func (s *Service) ls(ctx *kernel.Context, args []string) error {
	long := false
	var target string
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			if a == "-l" {
				long = true
				continue
			}
			return errors.New("usage: ls [-l] [path]")
		}
		if target != "" {
			return errors.New("usage: ls [-l] [path]")
		}
		target = a
	}
	if target == "" {
		target = "."
	}
	path := s.absPath(target)

	ents, err := s.vfsClient().List(ctx, path)
	if err != nil {
		return err
	}

	sort.Slice(ents, func(i, j int) bool { return ents[i].Name < ents[j].Name })
	for _, e := range ents {
		name := e.Name
		mode := "----------"
		if e.Type == proto.VFSEntryDir {
			mode = "drwxr-xr-x"
		} else if e.Type == proto.VFSEntryFile {
			mode = "-rw-r--r--"
		} else {
			mode = "?---------"
		}

		if !long {
			if err := s.printString(ctx, name+"\n"); err != nil {
				return err
			}
			continue
		}

		if err := s.printString(ctx, fmt.Sprintf("%s %5d %s\n", mode, e.Size, name)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) mkdir(ctx *kernel.Context, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: mkdir <path>")
	}
	return s.vfsClient().Mkdir(ctx, s.absPath(args[0]))
}

func (s *Service) touch(ctx *kernel.Context, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: touch <path>")
	}
	_, err := s.vfsClient().Write(ctx, s.absPath(args[0]), proto.VFSWriteAppend, nil)
	return err
}

func (s *Service) cp(ctx *kernel.Context, args []string) error {
	if len(args) != 2 {
		return errors.New("usage: cp <src> <dst>")
	}
	src := s.absPath(args[0])
	dst := s.absPath(args[1])

	const maxRead = kernel.MaxMessageBytes - 11
	var off uint32
	var buf []byte
	for {
		b, eof, err := s.vfsClient().ReadAt(ctx, src, off, maxRead)
		if err != nil {
			return err
		}
		if len(b) > 0 {
			buf = append(buf, b...)
			off += uint32(len(b))
			if len(buf) > 512*1024 {
				return errors.New("file too large")
			}
		}
		if eof {
			break
		}
	}
	_, err := s.vfsClient().Write(ctx, dst, proto.VFSWriteTruncate, buf)
	return err
}

func (s *Service) stat(ctx *kernel.Context, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: stat <path>")
	}
	typ, size, err := s.vfsClient().Stat(ctx, s.absPath(args[0]))
	if err != nil {
		return err
	}
	t := "?"
	switch typ {
	case proto.VFSEntryFile:
		t = "file"
	case proto.VFSEntryDir:
		t = "dir"
	}
	return s.printString(ctx, fmt.Sprintf("%s size=%d\n", t, size))
}

func (s *Service) cat(ctx *kernel.Context, args []string, redir redirection) error {
	if len(args) != 1 {
		return errors.New("usage: cat <path>")
	}
	path := s.absPath(args[0])

	const maxRead = kernel.MaxMessageBytes - 11
	var off uint32
	var buf []byte
	for {
		b, eof, err := s.vfsClient().ReadAt(ctx, path, off, maxRead)
		if err != nil {
			return err
		}
		if len(b) == 0 && eof {
			break
		}

		if len(b) > 0 {
			if redir.Path != "" {
				buf = append(buf, b...)
			} else {
				if err := s.writeBytes(ctx, b); err != nil {
					return err
				}
			}
			off += uint32(len(b))
		}
		if eof {
			break
		}
	}

	if redir.Path != "" {
		redir.Path = s.absPath(redir.Path)
		mode := proto.VFSWriteTruncate
		if redir.Append {
			mode = proto.VFSWriteAppend
		}
		_, err := s.vfsClient().Write(ctx, redir.Path, mode, buf)
		return err
	}
	return nil
}

func (s *Service) put(ctx *kernel.Context, args []string) error {
	if len(args) < 2 {
		return errors.New("usage: put <path> <data...>")
	}
	path := s.absPath(args[0])
	data := []byte(strings.Join(args[1:], " "))
	_, err := s.vfsClient().Write(ctx, path, proto.VFSWriteTruncate, data)
	return err
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

func (s *Service) echo(ctx *kernel.Context, args []string, redir redirection) error {
	if len(args) == 0 {
		if redir.Path == "" {
			return s.printString(ctx, "\n")
		}
		return nil
	}

	out := strings.Join(args, " ") + "\n"
	if redir.Path == "" {
		return s.printString(ctx, out)
	}

	redir.Path = s.absPath(redir.Path)
	mode := proto.VFSWriteTruncate
	if redir.Append {
		mode = proto.VFSWriteAppend
	}
	_, err := s.vfsClient().Write(ctx, redir.Path, mode, []byte(out))
	return err
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
	"cat",
	"cd",
	"clear",
	"cp",
	"echo",
	"help",
	"ls",
	"log",
	"mkdir",
	"panic",
	"put",
	"pwd",
	"rtdemo",
	"scrollback",
	"stat",
	"ticks",
	"touch",
	"uname",
	"uptime",
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
	{Name: "cat", Desc: "Print a file."},
	{Name: "ls", Desc: "List directory entries."},
	{Name: "pwd", Desc: "Print current directory."},
	{Name: "cd", Desc: "Change current directory."},
	{Name: "mkdir", Desc: "Create a directory."},
	{Name: "touch", Desc: "Create file if missing."},
	{Name: "cp", Desc: "Copy a file."},
	{Name: "put", Desc: "Write bytes to a file."},
	{Name: "stat", Desc: "Show file metadata."},
	{Name: "ticks", Desc: "Show current kernel tick counter."},
	{Name: "uptime", Desc: "Show uptime (ticks)."},
	{Name: "version", Desc: "Show build version."},
	{Name: "uname", Desc: "Show system information (try -a)."},
	{Name: "panic", Desc: "Panic the shell task (test)."},
	{Name: "log", Desc: "Send a log line to logger service."},
	{Name: "scrollback", Desc: "Show the last N output lines."},
	{Name: "rtdemo", Desc: "Toggle raytracing demo (Ctrl+G or `rtdemo`)."},
}

const scrollbackMaxLines = 200
