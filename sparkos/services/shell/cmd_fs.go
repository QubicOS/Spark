package shell

import (
	"errors"
	"fmt"
	"path"
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
		{Name: "mkdir", Usage: "mkdir [-p] <path...>", Desc: "Create directories.", Run: cmdMkdir},
		{Name: "rmdir", Usage: "rmdir <path...>", Desc: "Remove empty directories.", Run: cmdRmdir},
		{Name: "touch", Usage: "touch <path...>", Desc: "Create file if missing.", Run: cmdTouch},
		{Name: "cp", Usage: "cp <src> <dst>", Desc: "Copy a file.", Run: cmdCp},
		{Name: "mv", Usage: "mv <src> <dst>", Desc: "Rename (move) a path.", Run: cmdMv},
		{Name: "rm", Usage: "rm [-rf] <path...>", Desc: "Remove files or directories.", Run: cmdRm},
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
func cmdRmdir(ctx *kernel.Context, s *Service, args []string, _ redirection) error {
	return s.rmdir(ctx, args)
}
func cmdTouch(ctx *kernel.Context, s *Service, args []string, _ redirection) error {
	return s.touch(ctx, args)
}
func cmdCp(ctx *kernel.Context, s *Service, args []string, _ redirection) error {
	return s.cp(ctx, args)
}
func cmdMv(ctx *kernel.Context, s *Service, args []string, _ redirection) error {
	return s.mv(ctx, args)
}
func cmdRm(ctx *kernel.Context, s *Service, args []string, _ redirection) error {
	return s.rm(ctx, args)
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
	dirPath := s.absPath(target)

	ents, err := s.vfsClient().List(ctx, dirPath)
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
	pFlag := false
	var paths []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			if a == "-p" {
				pFlag = true
				continue
			}
			return errors.New("usage: mkdir [-p] <path...>")
		}
		paths = append(paths, a)
	}
	if len(paths) == 0 {
		return errors.New("usage: mkdir [-p] <path...>")
	}

	for _, p := range paths {
		abs := s.absPath(p)
		if pFlag {
			if err := s.mkdirAll(ctx, abs); err != nil {
				return err
			}
			continue
		}
		if err := s.vfsClient().Mkdir(ctx, abs); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) mkdirAll(ctx *kernel.Context, abs string) error {
	abs = cleanPath(abs)
	if abs == "/" {
		return nil
	}

	parts := strings.Split(strings.TrimPrefix(abs, "/"), "/")
	var cur string
	for _, p := range parts {
		if p == "" {
			continue
		}
		cur = cleanPath(cur + "/" + p)
		if err := s.vfsClient().Mkdir(ctx, cur); err != nil {
			typ, _, serr := s.vfsClient().Stat(ctx, cur)
			if serr == nil && typ == proto.VFSEntryDir {
				continue
			}
			return err
		}
	}
	return nil
}

func (s *Service) rmdir(ctx *kernel.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: rmdir <path...>")
	}
	for _, a := range args {
		p := s.absPath(a)
		typ, _, err := s.vfsClient().Stat(ctx, p)
		if err != nil {
			return err
		}
		if typ != proto.VFSEntryDir {
			return errors.New("not a directory")
		}
		if err := s.vfsClient().Remove(ctx, p); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) touch(ctx *kernel.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: touch <path...>")
	}
	for _, a := range args {
		_, err := s.vfsClient().Write(ctx, s.absPath(a), proto.VFSWriteAppend, nil)
		if err != nil {
			return err
		}
	}
	return nil
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

func (s *Service) mv(ctx *kernel.Context, args []string) error {
	if len(args) != 2 {
		return errors.New("usage: mv <src> <dst>")
	}
	src := s.absPath(args[0])
	dst := s.absPath(args[1])

	dstType, _, err := s.vfsClient().Stat(ctx, dst)
	if err == nil && dstType == proto.VFSEntryDir {
		dst = cleanPath(path.Join(dst, path.Base(src)))
	}
	return s.vfsClient().Rename(ctx, src, dst)
}

func (s *Service) rm(ctx *kernel.Context, args []string) error {
	recursive := false
	force := false

	var paths []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			for _, ch := range strings.TrimPrefix(a, "-") {
				switch ch {
				case 'r', 'R':
					recursive = true
				case 'f':
					force = true
				default:
					return errors.New("usage: rm [-rf] <path...>")
				}
			}
			continue
		}
		paths = append(paths, a)
	}
	if len(paths) == 0 {
		return errors.New("usage: rm [-rf] <path...>")
	}

	for _, a := range paths {
		p := s.absPath(a)
		if p == "/" {
			return errors.New("refusing to remove /")
		}
		if err := s.rmPath(ctx, p, recursive); err != nil {
			if force && strings.Contains(err.Error(), proto.ErrNotFound.String()) {
				continue
			}
			return err
		}
	}
	return nil
}

func (s *Service) rmPath(ctx *kernel.Context, abs string, recursive bool) error {
	typ, _, err := s.vfsClient().Stat(ctx, abs)
	if err != nil {
		return err
	}
	if typ == proto.VFSEntryDir {
		if !recursive {
			return errors.New("is a directory")
		}

		ents, err := s.vfsClient().List(ctx, abs)
		if err != nil {
			return err
		}
		for _, e := range ents {
			child := cleanPath(path.Join(abs, e.Name))
			if err := s.rmPath(ctx, child, recursive); err != nil {
				return err
			}
		}
	}
	return s.vfsClient().Remove(ctx, abs)
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
