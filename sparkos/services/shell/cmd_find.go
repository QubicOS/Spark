package shell

import (
	"errors"
	"path"
	"sort"

	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

func cmdFind(ctx *kernel.Context, s *Service, args []string, _ redirection) error {
	start := "."
	if len(args) == 1 {
		start = args[0]
	} else if len(args) > 1 {
		return errors.New("usage: find [path]")
	}

	root := s.absPath(start)
	return s.findWalk(ctx, root)
}

func (s *Service) findWalk(ctx *kernel.Context, abs string) error {
	typ, _, err := s.vfsClient().Stat(ctx, abs)
	if err != nil {
		return err
	}
	_ = s.printString(ctx, abs+"\n")
	if typ != proto.VFSEntryDir {
		return nil
	}

	ents, err := s.vfsClient().List(ctx, abs)
	if err != nil {
		return err
	}
	sort.Slice(ents, func(i, j int) bool { return ents[i].Name < ents[j].Name })
	for _, e := range ents {
		child := cleanPath(path.Join(abs, e.Name))
		if err := s.findWalk(ctx, child); err != nil {
			return err
		}
	}
	return nil
}
