package shell

import (
	"strings"
	"unicode/utf8"

	"spark/sparkos/internal/userdb"
	"spark/sparkos/kernel"
)

func (s *Service) beginSu(ctx *kernel.Context, target string) {
	s.suActive = true
	s.suTarget = strings.TrimSpace(target)
	s.suBuf = wipeBytes(s.suBuf)
	s.suFails = 0
	s.suBlock = 0
	s.utf8buf = s.utf8buf[:0]

	_ = s.writeString(ctx, "password: ")
}

func (s *Service) cancelSu(ctx *kernel.Context) {
	s.suActive = false
	s.suTarget = ""
	s.suBuf = wipeBytes(s.suBuf)
	s.utf8buf = s.utf8buf[:0]
	s.suBlock = 0
	s.suFails = 0
	_ = s.prompt(ctx)
}

func (s *Service) handleSuInput(ctx *kernel.Context, b []byte) {
	now := ctx.NowTick()
	if s.suBlock != 0 && now < s.suBlock {
		s.utf8buf = s.utf8buf[:0]
		return
	}

	s.utf8buf = append(s.utf8buf, b...)
	b = s.utf8buf

	for len(b) > 0 {
		if b[0] == 0x1b {
			// Ignore escape sequences in password prompt.
			if len(b) == 1 {
				b = b[1:]
				continue
			}
			if len(b) < 3 {
				s.utf8buf = b
				return
			}
			if b[1] == '[' {
				b = b[3:]
				continue
			}
			b = b[1:]
			continue
		}

		switch b[0] {
		case '\r':
			b = b[1:]
		case '\n':
			b = b[1:]
			s.suSubmit(ctx)
		case 0x7f, 0x08:
			b = b[1:]
			s.suBackspace(ctx)
		case 0x03:
			// Ctrl+C.
			b = b[1:]
			_ = s.writeString(ctx, "\n")
			s.cancelSu(ctx)
			return
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
			if len(s.suBuf) >= 64 {
				continue
			}
			if r > 0x7e {
				continue
			}
			s.suBuf = append(s.suBuf, byte(r))
			_ = s.writeString(ctx, "*")
		}
	}

	s.utf8buf = s.utf8buf[:0]
}

func (s *Service) suBackspace(ctx *kernel.Context) {
	if len(s.suBuf) == 0 {
		return
	}
	s.suBuf = s.suBuf[:len(s.suBuf)-1]
	_ = s.writeString(ctx, "\b \b")
}

func (s *Service) suSubmit(ctx *kernel.Context) {
	_ = s.writeString(ctx, "\n")

	target := strings.TrimSpace(s.suTarget)
	if target == "" {
		s.cancelSu(ctx)
		return
	}

	users, ok, err := s.loadUsers(ctx)
	if err != nil || !ok {
		s.suBuf = wipeBytes(s.suBuf)
		s.suBuf = s.suBuf[:0]
		_ = s.writeString(ctx, "su: users db unavailable\n")
		s.cancelSu(ctx)
		return
	}
	rec, found := userdb.Find(users, target)
	if !found {
		s.suBuf = wipeBytes(s.suBuf)
		s.suBuf = s.suBuf[:0]
		_ = s.writeString(ctx, "su: unknown user\n")
		s.cancelSu(ctx)
		return
	}

	ok = rec.VerifyPassword(s.suBuf)
	s.suBuf = wipeBytes(s.suBuf)
	s.suBuf = s.suBuf[:0]
	if ok {
		s.applyUser(rec)
		s.suActive = false
		s.suTarget = ""
		s.suFails = 0
		s.suBlock = 0
		_ = s.writeString(ctx, s.tabStatusLine())
		_ = s.prompt(ctx)
		return
	}

	s.suFails++
	delay := uint64(1000)
	if s.suFails > 1 {
		delay = uint64(s.suFails) * 1000
		if delay > 8000 {
			delay = 8000
		}
	}
	s.suBlock = ctx.NowTick() + delay
	_ = s.writeString(ctx, "Login failed.\n")
	_ = s.writeString(ctx, "password: ")
}
