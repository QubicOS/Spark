package shell

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"spark/internal/buildinfo"
	vfsclient "spark/sparkos/client/vfs"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

type Service struct {
	inCap   kernel.Capability
	termCap kernel.Capability
	logCap  kernel.Capability
	vfsCap  kernel.Capability
	timeCap kernel.Capability
	muxCap  kernel.Capability

	vfs *vfsclient.Client
	reg *registry

	tabs   []tabState
	tabIdx int

	line   []rune
	cursor int

	utf8buf []byte

	history []string
	histPos int
	scratch []rune

	scrollback []string

	cwd string

	hint  string
	ghost string
	cands []string
	best  string

	suppressPromptOnce bool

	authed bool

	authHaveShadow bool
	authRec        shadowRecord
	authSetup      bool
	authStage      authStage
	authUser       string

	authBuf    []byte
	authPass1  []byte
	authFails  int
	authBlock  uint64
	authBanner bool
}

func New(inCap kernel.Capability, termCap kernel.Capability, logCap kernel.Capability, vfsCap kernel.Capability, timeCap kernel.Capability, muxCap kernel.Capability) *Service {
	return &Service{inCap: inCap, termCap: termCap, logCap: logCap, vfsCap: vfsCap, timeCap: timeCap, muxCap: muxCap}
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

	s.initTabsIfNeeded()
	if s.reg == nil {
		if err := s.initRegistry(); err != nil {
			_ = s.printString(ctx, "shell: init: "+err.Error()+"\n")
			return
		}
	}

	_ = s.printString(ctx, "\x1b[0m")
	_ = s.printString(ctx, s.banner())
	s.authBanner = true
	s.beginAuth(ctx)

	for msg := range ch {
		switch proto.Kind(msg.Kind) {
		case proto.MsgTermInput:
			if s.authed {
				s.handleInput(ctx, msg.Payload())
			} else {
				s.handleAuthInput(ctx, msg.Payload())
			}
		case proto.MsgAppControl:
			active, ok := proto.DecodeAppControlPayload(msg.Payload())
			if !ok {
				continue
			}
			s.handleFocus(ctx, active)
		}
	}
}

type tabState struct {
	line   []rune
	cursor int

	history []string
	histPos int
	scratch []rune

	scrollback []string

	cwd string

	name string

	hint  string
	ghost string
	cands []string
	best  string
}

func (s *Service) initTabsIfNeeded() {
	if len(s.tabs) == 0 {
		if s.cwd == "" {
			s.cwd = "/"
		}
		s.tabs = []tabState{{
			line:   s.line,
			cursor: s.cursor,

			history: s.history,
			histPos: s.histPos,
			scratch: s.scratch,

			scrollback: s.scrollback,

			cwd: s.cwd,

			name: "",

			hint:  s.hint,
			ghost: s.ghost,
			cands: s.cands,
			best:  s.best,
		}}
		s.tabIdx = 0
	}
	if s.tabIdx < 0 || s.tabIdx >= len(s.tabs) {
		s.tabIdx = 0
	}
}

func (s *Service) stashTab(i int) {
	if i < 0 || i >= len(s.tabs) {
		return
	}
	s.tabs[i] = tabState{
		line:   s.line,
		cursor: s.cursor,

		history: s.history,
		histPos: s.histPos,
		scratch: s.scratch,

		scrollback: s.scrollback,

		cwd: s.cwd,

		name: s.tabs[i].name,

		hint:  s.hint,
		ghost: s.ghost,
		cands: s.cands,
		best:  s.best,
	}
}

func (s *Service) restoreTab(i int) {
	if i < 0 || i >= len(s.tabs) {
		return
	}
	t := s.tabs[i]
	s.line = t.line
	s.cursor = t.cursor

	s.history = t.history
	s.histPos = t.histPos
	s.scratch = t.scratch

	s.scrollback = t.scrollback

	s.cwd = t.cwd
	s.tabs[i].name = t.name

	s.hint = t.hint
	s.ghost = t.ghost
	s.cands = t.cands
	s.best = t.best
}

func (s *Service) currentTab() (idx int, total int) {
	return s.tabIdx, len(s.tabs)
}

func (s *Service) newTab(ctx *kernel.Context, suppressPrompt bool) {
	s.initTabsIfNeeded()
	s.stashTab(s.tabIdx)
	s.tabs = append(s.tabs, tabState{cwd: s.cwd})
	_ = s.switchTab(ctx, len(s.tabs)-1, suppressPrompt)
}

func (s *Service) closeTab(ctx *kernel.Context, suppressPrompt bool) {
	s.initTabsIfNeeded()
	if len(s.tabs) <= 1 {
		_ = s.printString(ctx, "tab: cannot close the last tab\n")
		return
	}
	s.stashTab(s.tabIdx)
	i := s.tabIdx
	copy(s.tabs[i:], s.tabs[i+1:])
	s.tabs = s.tabs[:len(s.tabs)-1]
	if i >= len(s.tabs) {
		i = len(s.tabs) - 1
	}
	_ = s.switchTab(ctx, i, suppressPrompt)
}

func (s *Service) nextTab(ctx *kernel.Context, suppressPrompt bool) {
	s.initTabsIfNeeded()
	if len(s.tabs) <= 1 {
		return
	}
	_ = s.switchTab(ctx, (s.tabIdx+1)%len(s.tabs), suppressPrompt)
}

func (s *Service) prevTab(ctx *kernel.Context, suppressPrompt bool) {
	s.initTabsIfNeeded()
	if len(s.tabs) <= 1 {
		return
	}
	n := len(s.tabs)
	_ = s.switchTab(ctx, (s.tabIdx+n-1)%n, suppressPrompt)
}

func (s *Service) switchTab(ctx *kernel.Context, idx int, suppressPrompt bool) bool {
	s.initTabsIfNeeded()
	if idx < 0 || idx >= len(s.tabs) || idx == s.tabIdx {
		return false
	}
	s.stashTab(s.tabIdx)
	s.tabIdx = idx
	s.restoreTab(s.tabIdx)
	s.renderTab(ctx)
	if suppressPrompt {
		s.suppressPromptOnce = true
	}
	return true
}

func (s *Service) tabStatusLine() string {
	tab := s.tabIdx + 1
	total := len(s.tabs)
	label := s.cwd
	if s.tabIdx >= 0 && s.tabIdx < len(s.tabs) {
		if n := strings.TrimSpace(s.tabs[s.tabIdx].name); n != "" {
			label = n
		}
	}
	return fmt.Sprintf("\x1b[38;5;245m[tab %d/%d] %s\x1b[0m\n", tab, total, label)
}

func (s *Service) renderTab(ctx *kernel.Context) {
	_ = s.sendToTerm(ctx, proto.MsgTermClear, nil)
	_ = s.writeString(ctx, "\x1b[0m")

	_ = s.writeString(ctx, s.tabStatusLine())

	const maxReplay = 60
	start := len(s.scrollback) - maxReplay
	if start < 0 {
		start = 0
	}
	for _, ln := range s.scrollback[start:] {
		_ = s.writeString(ctx, ln+"\n")
	}

	_ = s.redrawLine(ctx)
	_ = s.sendToTerm(ctx, proto.MsgTermRefresh, nil)
}

func (s *Service) banner() string {
	bolt := "\x1b[38;5;220m"
	info := "\x1b[38;5;39m"
	dim := "\x1b[38;5;245m"
	reset := "\x1b[0m"

	b := buildinfo.Short()
	if buildinfo.Date != "" && buildinfo.Date != "unknown" {
		b += " " + buildinfo.Date
	}

	const logoW = 7
	const textCol = 22

	// Left: a small "bolt" logo, right: brief intro.
	return "" +
		info + "Welcome to SparkOS" + reset + "\n" +
		bolt + "   /\\   " + reset + dim + "Spark is a personal operating system project" + reset + "\n" +
		bolt + "  /  \\  " + reset + dim + "focused on building a small and usable" + reset + "\n" +
		bolt + " / /\\ \\ " + reset + dim + "system for embedded devices." + reset + "\n" +
		bolt + " \\ \\/ / " + reset + dim + "Build: " + b + reset + "\n" +
		bolt + "  \\__/  " + reset + dim + "Type: help" + reset + "\n\n"
}

func (s *Service) handleFocus(ctx *kernel.Context, active bool) {
	if !active {
		return
	}
	s.utf8buf = s.utf8buf[:0]
	if !s.authed {
		_ = s.sendToTerm(ctx, proto.MsgTermClear, nil)
		_ = s.writeString(ctx, "\x1b[0m")
		if !s.authBanner {
			_ = s.writeString(ctx, s.banner())
			s.authBanner = true
		} else {
			_ = s.writeString(ctx, s.banner())
		}
		_ = s.redrawAuth(ctx)
		_ = s.sendToTerm(ctx, proto.MsgTermRefresh, nil)
	} else {
		s.initTabsIfNeeded()
		s.renderTab(ctx)
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
			case escF1:
				s.prevTab(ctx, false)
			case escF2:
				s.nextTab(ctx, false)
			case escF3:
				s.newTab(ctx, false)
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
		if len(s.history) > historyMaxEntries {
			excess := len(s.history) - historyMaxEntries
			copy(s.history, s.history[excess:])
			s.history = s.history[:historyMaxEntries]
		}
	}
	s.histPos = len(s.history)

	args, redirect, ok := parseArgs(line)
	if !ok || len(args) == 0 {
		_ = s.prompt(ctx)
		return
	}
	name := args[0]
	args = args[1:]

	cmd, ok := s.reg.resolve(name)
	if !ok {
		_ = s.printString(ctx, "unknown command: "+name+"\n")
		_ = s.prompt(ctx)
		return
	}
	if err := cmd.Run(ctx, s, args, redirect); err != nil {
		_ = s.printString(ctx, cmd.Name+": "+err.Error()+"\n")
	}
	if redirect.Path != "" && cmd.Name != "echo" && cmd.Name != "cat" {
		_ = s.printString(ctx, "redirect: not supported for "+cmd.Name+"\n")
	}

	if s.suppressPromptOnce {
		s.suppressPromptOnce = false
		return
	}
	_ = s.prompt(ctx)
}
