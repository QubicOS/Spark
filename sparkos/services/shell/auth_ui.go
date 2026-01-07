package shell

import (
	"crypto/rand"
	"crypto/subtle"
	"strings"
	"unicode/utf8"

	"spark/sparkos/kernel"
)

type authStage uint8

const (
	authUser authStage = iota
	authPass
	authPassConfirm
)

func (s *Service) beginAuth(ctx *kernel.Context) {
	rec, ok, err := s.loadShadow(ctx)
	if err != nil {
		_ = s.writeString(ctx, "auth: "+err.Error()+"\n")
	}
	s.authHaveShadow = ok
	s.authRec = rec
	s.authSetup = !ok
	s.authStage = authUser
	s.authUser = ""
	s.authBuf = s.authBuf[:0]
	s.authPass1 = wipeBytes(s.authPass1)
	s.authFails = 0
	s.authBlock = 0

	_ = s.writeString(ctx, "\n")
	if s.authSetup {
		_ = s.writeString(ctx, "Setup: create root password.\n")
	}
	_ = s.writeString(ctx, "login: ")
}

func (s *Service) handleAuthInput(ctx *kernel.Context, b []byte) {
	now := ctx.NowTick()
	if s.authBlock != 0 && now < s.authBlock {
		return
	}

	s.utf8buf = append(s.utf8buf, b...)
	b = s.utf8buf

	for len(b) > 0 {
		if b[0] == 0x1b {
			// Ignore escape sequences in login.
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
			s.authSubmit(ctx)
		case 0x7f, 0x08:
			b = b[1:]
			s.authBackspace(ctx)
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
			if len(s.authBuf) >= 64 {
				continue
			}
			if r > 0x7e {
				continue
			}
			s.authBuf = append(s.authBuf, byte(r))
			if s.authStage == authPass || s.authStage == authPassConfirm {
				_ = s.writeString(ctx, "*")
			} else {
				_ = s.writeString(ctx, string(r))
			}
		}
	}

	s.utf8buf = s.utf8buf[:0]
}

func (s *Service) authBackspace(ctx *kernel.Context) {
	if len(s.authBuf) == 0 {
		return
	}
	s.authBuf = s.authBuf[:len(s.authBuf)-1]
	_ = s.writeString(ctx, "\b \b")
}

func (s *Service) authSubmit(ctx *kernel.Context) {
	_ = s.writeString(ctx, "\n")

	switch s.authStage {
	case authUser:
		u := strings.TrimSpace(string(s.authBuf))
		s.authBuf = wipeBytes(s.authBuf)
		s.authBuf = s.authBuf[:0]
		if u == "" {
			u = "root"
		}
		s.authUser = u
		_ = s.writeString(ctx, "password: ")
		s.authStage = authPass
		return

	case authPass:
		if s.authUser != "root" {
			s.authBuf = wipeBytes(s.authBuf)
			s.authBuf = s.authBuf[:0]
			s.authFail(ctx)
			return
		}

		if s.authSetup {
			s.authPass1 = wipeBytes(s.authPass1)
			s.authPass1 = append(s.authPass1, s.authBuf...)
			s.authBuf = wipeBytes(s.authBuf)
			s.authBuf = s.authBuf[:0]
			_ = s.writeString(ctx, "confirm: ")
			s.authStage = authPassConfirm
			return
		}

		if !s.authHaveShadow {
			s.authBuf = wipeBytes(s.authBuf)
			s.authBuf = s.authBuf[:0]
			s.authFail(ctx)
			return
		}

		ok := verifyPassword(s.authRec, s.authBuf)
		s.authBuf = wipeBytes(s.authBuf)
		s.authBuf = s.authBuf[:0]
		if !ok {
			s.authFail(ctx)
			return
		}
		s.authSuccess(ctx)
		return

	case authPassConfirm:
		pass2 := s.authBuf
		ok := len(s.authPass1) == len(pass2) && subtle.ConstantTimeCompare(s.authPass1, pass2) == 1
		s.authBuf = wipeBytes(s.authBuf)
		s.authBuf = s.authBuf[:0]
		if !ok {
			s.authPass1 = wipeBytes(s.authPass1)
			_ = s.writeString(ctx, "mismatch, try again.\n")
			_ = s.writeString(ctx, "password: ")
			s.authStage = authPass
			return
		}

		rec := shadowRecord{user: "root", scheme: shadowSchemePBKDF2SHA256, iter: defaultPBKDF2Iters}
		rec.salt = s.makeSalt(ctx)
		rec.hash = hashPasswordPBKDF2SHA256(rec.iter, rec.salt, s.authPass1)
		s.authPass1 = wipeBytes(s.authPass1)

		if err := s.writeShadow(ctx, rec); err != nil {
			_ = s.writeString(ctx, "auth: write shadow: "+err.Error()+"\n")
			s.authFail(ctx)
			return
		}

		s.authRec = rec
		s.authHaveShadow = true
		s.authSetup = false
		s.authSuccess(ctx)
		return
	}
}

func (s *Service) authFail(ctx *kernel.Context) {
	s.authFails++
	delay := uint64(1000)
	if s.authFails > 1 {
		delay = uint64(1000 * s.authFails)
		if delay > 8000 {
			delay = 8000
		}
	}
	s.authBlock = ctx.NowTick() + delay
	_ = s.writeString(ctx, "Login failed.\n")
	_ = s.writeString(ctx, "login: ")
	s.authStage = authUser
	s.authUser = ""
	s.authBuf = s.authBuf[:0]
	s.authPass1 = wipeBytes(s.authPass1)
}

func (s *Service) authSuccess(ctx *kernel.Context) {
	s.authed = true
	s.authBuf = nil
	s.authPass1 = wipeBytes(s.authPass1)
	s.authBanner = false

	_ = s.writeString(ctx, "Welcome, root.\n\n")
	_ = s.prompt(ctx)
}

func (s *Service) redrawAuth(ctx *kernel.Context) error {
	// Re-render the current auth prompt line best-effort.
	switch s.authStage {
	case authUser:
		_ = s.writeString(ctx, "\nlogin: ")
		_ = s.writeString(ctx, string(s.authBuf))
	case authPass:
		_ = s.writeString(ctx, "\npassword: ")
		_ = s.writeString(ctx, strings.Repeat("*", len(s.authBuf)))
	case authPassConfirm:
		_ = s.writeString(ctx, "\nconfirm: ")
		_ = s.writeString(ctx, strings.Repeat("*", len(s.authBuf)))
	}
	return nil
}

func wipeBytes(b []byte) []byte {
	for i := range b {
		b[i] = 0
	}
	return b[:0]
}

func (s *Service) makeSalt(ctx *kernel.Context) [16]byte {
	var out [16]byte
	if _, err := rand.Read(out[:]); err == nil {
		return out
	}

	seed := uint32(ctx.NowTick()) ^ uint32(s.authFails*0x9e3779b9)
	if seed == 0 {
		seed = 0x12345678
	}
	x := seed
	for i := 0; i < len(out); i++ {
		// xorshift32.
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		out[i] = byte(x)
	}
	return out
}
