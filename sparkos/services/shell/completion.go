package shell

import (
	"strings"

	"spark/sparkos/kernel"
)

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

func (s *Service) commandMatches(prefix string) []string {
	if s.reg == nil {
		return nil
	}
	return s.reg.matches(prefix)
}
