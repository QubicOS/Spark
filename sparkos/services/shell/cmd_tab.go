package shell

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"spark/sparkos/kernel"
)

func cmdTab(ctx *kernel.Context, s *Service, args []string, _ redirection) error {
	s.initTabsIfNeeded()
	s.stashTab(s.tabIdx)

	if len(args) == 0 {
		i, n := s.currentTab()
		return s.printString(ctx, fmt.Sprintf("tab %d/%d\n", i+1, n))
	}

	switch args[0] {
	case "new":
		s.newTab(ctx, true)
		return nil
	case "close":
		s.closeTab(ctx, true)
		return nil
	case "next":
		s.nextTab(ctx, true)
		return nil
	case "prev":
		s.prevTab(ctx, true)
		return nil
	case "list":
		var b strings.Builder
		for i, t := range s.tabs {
			mark := " "
			if i == s.tabIdx {
				mark = "*"
			}
			cwd := t.cwd
			if cwd == "" {
				cwd = "/"
			}
			fmt.Fprintf(&b, "%s %d\t%s\n", mark, i+1, cwd)
		}
		return s.printString(ctx, b.String())
	case "go":
		if len(args) != 2 {
			return errors.New("usage: tab go <n>")
		}
		n, err := strconv.Atoi(args[1])
		if err != nil || n <= 0 {
			return fmt.Errorf("tab go: invalid tab %q", args[1])
		}
		_ = s.switchTab(ctx, n-1, true)
		return nil
	default:
		return errors.New("usage: tab [new|close|next|prev|list|go <n>]")
	}
}
