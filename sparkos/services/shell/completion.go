package shell

import (
	"fmt"
	"strings"

	"spark/sparkos/kernel"
)

type completionMode uint8

const (
	completionNone completionMode = iota
	completionCommand
	completionArg
)

const (
	popupBGANSI    = "\x1b[48;5;238m"
	popupFGANSI    = "\x1b[38;5;255m"
	popupResetANSI = "\x1b[0m"
)

func (s *Service) tab(ctx *kernel.Context) {
	s.updateCompletion()
	if s.cursor != len(s.line) || s.cursor == 0 || len(s.cands) == 0 {
		return
	}

	prefix := s.compToken
	matches := s.cands

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

	items := make([]string, 0, len(matches))
	for _, m := range matches {
		if m == s.best {
			continue
		}
		items = append(items, m)
	}
	if len(items) == 0 {
		return
	}

	_ = s.writeString(ctx, "\n")
	_ = s.writeCompletionPopup(ctx, items)
	_ = s.redrawLine(ctx)
}

func (s *Service) commandMatches(prefix string) []string {
	if s.reg == nil {
		return nil
	}
	return s.reg.matches(prefix)
}

func (s *Service) argMatches(cmdName string, argIndex int, prefix string, argsBefore []string) []string {
	if cmdName == "help" && argIndex == 0 && s.reg != nil {
		return s.reg.matches(prefix)
	}

	cands := argCandidates(cmdName, argIndex, argsBefore)
	if len(cands) == 0 {
		return nil
	}
	out := make([]string, 0, len(cands))
	for _, c := range cands {
		if strings.HasPrefix(c, prefix) {
			out = append(out, c)
		}
	}
	return out
}

func argCandidates(cmdName string, argIndex int, argsBefore []string) []string {
	switch cmdName {
	case "focus":
		if argIndex == 0 {
			return []string{"app", "shell", "toggle"}
		}
	case "tab":
		if argIndex == 0 {
			return []string{"new", "close", "next", "prev", "name", "list", "go"}
		}
		_ = argsBefore
	case "free":
		if argIndex == 0 {
			return []string{"-h"}
		}
	case "uname":
		if argIndex == 0 {
			return []string{"-a"}
		}
	case "todo":
		if argIndex == 0 {
			return []string{"all", "open", "done", "search"}
		}
	case "rtdemo", "rtvoxel":
		if argIndex == 0 {
			return []string{"on", "off"}
		}
	}
	return nil
}

func (s *Service) updateCompletion() {
	s.hint = ""
	s.ghost = ""
	s.cands = nil
	s.best = ""
	s.compMode = completionNone
	s.compTokenStart = 0
	s.compToken = ""

	if s.reg == nil {
		return
	}
	if s.cursor != len(s.line) || s.cursor == 0 {
		return
	}

	line := string(s.line[:s.cursor])
	if strings.ContainsAny(line, "\"'\\") {
		return
	}

	fields := strings.Fields(line)
	trailingSpace := strings.HasSuffix(line, " ")

	if len(fields) == 0 {
		return
	}

	// Command completion.
	if len(fields) == 1 && !trailingSpace {
		prefix := fields[0]
		matches := s.commandMatches(prefix)
		if len(matches) == 0 {
			return
		}
		s.compMode = completionCommand
		s.compTokenStart = 0
		s.compToken = prefix
		s.cands = matches
		s.best = matches[0]
		if strings.HasPrefix(s.best, prefix) && s.best != prefix {
			s.ghost = s.best[len(prefix):]
		}

		cmdName := s.best
		if prefix == s.best {
			cmdName = prefix
		}
		if cmd, ok := s.reg.resolve(cmdName); ok && cmd.Usage != "" {
			s.hint = cmd.Usage
			return
		}
		if len(matches) > 1 {
			s.hint = fmt.Sprintf("Tab: complete (%d)", len(matches))
		}
		return
	}

	// Argument completion (limited to simple space-separated inputs).
	cmdName := fields[0]
	cmd, ok := s.reg.resolve(cmdName)
	if ok && cmd.Usage != "" {
		s.hint = cmd.Usage
	}

	var (
		argIndex   int
		prefix     string
		argsBefore []string
	)
	if trailingSpace {
		argIndex = len(fields) - 1
		prefix = ""
		argsBefore = fields[1:]
		s.compTokenStart = s.cursor
	} else {
		argIndex = len(fields) - 2
		prefix = fields[len(fields)-1]
		argsBefore = fields[1 : len(fields)-1]
		s.compTokenStart = strings.LastIndexByte(line, ' ') + 1
	}

	if argIndex < 0 {
		return
	}

	matches := s.argMatches(cmdName, argIndex, prefix, argsBefore)
	if len(matches) == 0 {
		return
	}

	s.compMode = completionArg
	s.compToken = prefix
	s.cands = matches
	s.best = matches[0]
	if strings.HasPrefix(s.best, prefix) && s.best != prefix {
		s.ghost = s.best[len(prefix):]
	}

	if s.hint == "" && len(matches) > 1 {
		s.hint = fmt.Sprintf("Tab: complete (%d)", len(matches))
	}
}

func (s *Service) writeCompletionPopup(ctx *kernel.Context, items []string) error {
	if len(items) == 0 {
		return nil
	}

	maxPopupRows := 4
	maxLen := 0
	for _, it := range items {
		n := len([]rune(it))
		if n > maxLen {
			maxLen = n
		}
	}
	if maxLen > 18 {
		maxLen = 18
	}
	colW := maxLen + 2
	if colW < 8 {
		colW = 8
	}

	const termColsApprox = 53
	cols := (termColsApprox - 2) / colW
	if cols < 1 {
		cols = 1
	}

	needRows := (len(items) + cols - 1) / cols
	if needRows > maxPopupRows {
		needRows = maxPopupRows
	}

	capItems := needRows * cols
	if capItems < len(items) {
		items = append(items[:capItems-1], "…")
	} else {
		items = items[:minInt(len(items), capItems)]
	}

	boxCols := cols*colW + 2

	for row := 0; row < needRows; row++ {
		line := make([]rune, boxCols)
		for i := range line {
			line[i] = ' '
		}

		for col := 0; col < cols; col++ {
			idx := row*cols + col
			if idx >= len(items) {
				break
			}
			it := []rune(items[idx])
			if len(it) > maxLen {
				it = it[:maxLen]
			}
			start := 1 + col*colW
			copy(line[start:start+len(it)], it)
		}

		if err := s.writeString(ctx, popupBGANSI+popupFGANSI+string(line)+popupResetANSI+"\n"); err != nil {
			return err
		}
	}
	return nil
}
