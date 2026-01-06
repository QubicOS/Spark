package shell

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	logclient "spark/sparkos/client/logger"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

func registerCoreCommands(r *registry) error {
	for _, cmd := range []command{
		{Name: "help", Usage: "help [command]", Desc: "Show available commands.", Run: cmdHelp},
		{Name: "clear", Usage: "clear", Desc: "Clear the terminal.", Run: cmdClear},
		{Name: "echo", Usage: "echo [args...]", Desc: "Print arguments.", Run: cmdEcho},
		{Name: "panic", Usage: "panic", Desc: "Panic the shell task (test).", Run: cmdPanic},
		{Name: "log", Usage: "log <line>", Desc: "Send a log line to logger service.", Run: cmdLog},
		{Name: "scrollback", Usage: "scrollback [n]", Desc: "Show the last N output lines.", Run: cmdScrollback},
	} {
		if err := r.register(cmd); err != nil {
			return err
		}
	}
	return nil
}

func cmdHelp(ctx *kernel.Context, s *Service, args []string, _ redirection) error {
	if s.reg == nil {
		return errors.New("help: no registry")
	}
	if len(args) == 0 {
		for _, name := range s.reg.names() {
			cmd, ok := s.reg.resolve(name)
			if !ok {
				continue
			}
			_ = s.printString(ctx, fmt.Sprintf("%-10s %s\n", cmd.Name, cmd.Desc))
		}
		return nil
	}
	if len(args) != 1 {
		return errors.New("usage: help [command]")
	}

	cmd, ok := s.reg.resolve(args[0])
	if !ok {
		return fmt.Errorf("unknown command: %s", args[0])
	}

	if cmd.Usage != "" {
		_ = s.printString(ctx, "usage: "+cmd.Usage+"\n")
	}
	if cmd.Desc != "" {
		_ = s.printString(ctx, cmd.Desc+"\n")
	}
	if len(cmd.Aliases) > 0 {
		_ = s.printString(ctx, "aliases: "+strings.Join(cmd.Aliases, ", ")+"\n")
	}
	return nil
}

func cmdClear(ctx *kernel.Context, s *Service, _ []string, _ redirection) error {
	_ = s.sendToTerm(ctx, proto.MsgTermClear, nil)
	return nil
}

func cmdEcho(ctx *kernel.Context, s *Service, args []string, redir redirection) error {
	return s.echo(ctx, args, redir)
}

func cmdPanic(_ *kernel.Context, _ *Service, _ []string, _ redirection) error {
	panic("shell panic")
}

func cmdLog(ctx *kernel.Context, s *Service, args []string, _ redirection) error {
	if len(args) == 0 {
		return errors.New("usage: log <line>")
	}
	logLine := strings.Join(args, " ")
	res := logclient.Log(ctx, s.logCap, logLine)
	if res != kernel.SendOK {
		return fmt.Errorf("logger: %s", res)
	}
	return nil
}

func cmdScrollback(ctx *kernel.Context, s *Service, args []string, _ redirection) error {
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
		return nil
	}
	for _, ln := range s.scrollback[start:] {
		_ = s.writeString(ctx, ln+"\n")
	}
	return nil
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
