package shell

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

func registerFSCommands(r *registry) error {
	for _, cmd := range []command{
		{Name: "ls", Usage: "ls [-l] [path]", Desc: "List directory entries.", Run: cmdLs},
		{Name: "pwd", Usage: "pwd", Desc: "Print current directory.", Run: cmdPwd},
		{Name: "cd", Usage: "cd [dir]", Desc: "Change current directory.", Run: cmdCd},
		{Name: "mkdir", Usage: "mkdir <path>", Desc: "Create a directory.", Run: cmdMkdir},
		{Name: "touch", Usage: "touch <path>", Desc: "Create file if missing.", Run: cmdTouch},
		{Name: "cp", Usage: "cp <src> <dst>", Desc: "Copy a file.", Run: cmdCp},
		{Name: "stat", Usage: "stat <path>", Desc: "Show file metadata.", Run: cmdStat},
		{Name: "cat", Usage: "cat <path>", Desc: "Print a file.", Run: cmdCat},
		{Name: "put", Usage: "put <path> <data...>", Desc: "Write bytes to a file.", Run: cmdPut},
	} {
		if err := r.register(cmd); err != nil {
			return err
		}
	}
	return nil
}

func cmdLs(ctx *kernel.Context, s *Service, args []string, _ redirection) error {
	return s.ls(ctx, args)
}
func cmdPwd(ctx *kernel.Context, s *Service, _ []string, _ redirection) error {
	return s.printString(ctx, s.cwd+"\n")
}
func cmdCd(ctx *kernel.Context, s *Service, args []string, _ redirection) error {
	return s.cd(ctx, args)
}
func cmdMkdir(ctx *kernel.Context, s *Service, args []string, _ redirection) error {
	return s.mkdir(ctx, args)
}
func cmdTouch(ctx *kernel.Context, s *Service, args []string, _ redirection) error {
	return s.touch(ctx, args)
}
func cmdCp(ctx *kernel.Context, s *Service, args []string, _ redirection) error {
	return s.cp(ctx, args)
}
func cmdStat(ctx *kernel.Context, s *Service, args []string, _ redirection) error {
	return s.stat(ctx, args)
}
func cmdCat(ctx *kernel.Context, s *Service, args []string, redir redirection) error {
	return s.cat(ctx, args, redir)
}
func cmdPut(ctx *kernel.Context, s *Service, args []string, _ redirection) error {
	return s.put(ctx, args)
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
