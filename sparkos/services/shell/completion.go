package shell

import (
	"fmt"
	"strings"

	"spark/sparkos/kernel"
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

func (s *Service) updateCompletion() {
	s.hint = ""
	s.ghost = ""
	s.cands = nil
	s.best = ""

	if s.reg == nil {
		return
	}
	if s.cursor != len(s.line) || s.cursor == 0 {
		return
	}
	if strings.IndexByte(string(s.line[:s.cursor]), ' ') >= 0 {
		return
	}

	prefix := string(s.line[:s.cursor])
	matches := s.commandMatches(prefix)
	if len(matches) == 0 {
		return
	}

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
