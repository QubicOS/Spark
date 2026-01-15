package shell

import (
	"encoding/hex"
	"errors"
	"strings"

	vfsclient "spark/sparkos/client/vfs"
	"spark/sparkos/internal/userdb"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

const (
	legacyShadowPath   = "/etc/shadow"
	legacyShadowMaxLen = 512

	defaultPBKDF2Iters = 20000
)

func (s *Service) ensureAuthVFS(ctx *kernel.Context) error {
	if s.vfs == nil && s.vfsCap.Valid() {
		s.vfs = vfsclient.New(s.vfsCap)
	}
	if s.vfs == nil {
		return errors.New("auth: vfs unavailable")
	}
	return nil
}

func (s *Service) loadUsers(ctx *kernel.Context) ([]userdb.Record, bool, error) {
	if err := s.ensureAuthVFS(ctx); err != nil {
		return nil, false, err
	}

	typ, size, err := s.vfs.Stat(ctx, userdb.UsersPath)
	if err != nil || typ != proto.VFSEntryFile {
		return nil, false, nil
	}
	if size == 0 {
		return nil, false, errors.New("auth: empty users db")
	}
	if size > userdb.MaxFileBytes {
		return nil, false, errors.New("auth: users db too large")
	}

	b, err := s.readFileLimited(ctx, userdb.UsersPath, userdb.MaxFileBytes)
	if err != nil {
		return nil, false, err
	}
	users, err := userdb.ParseUsersFile(b)
	if err != nil {
		return nil, false, err
	}
	return users, true, nil
}

func (s *Service) writeUsers(ctx *kernel.Context, users []userdb.Record) error {
	if err := s.ensureAuthVFS(ctx); err != nil {
		return err
	}
	if err := s.vfs.Mkdir(ctx, "/etc"); err != nil {
		_ = err
	}

	b, err := userdb.FormatUsersFile(users)
	if err != nil {
		return err
	}
	_, err = s.vfs.Write(ctx, userdb.UsersPath, proto.VFSWriteTruncate, b)
	return err
}

func (s *Service) readFileLimited(ctx *kernel.Context, path string, max int) ([]byte, error) {
	if s.vfs == nil {
		return nil, errors.New("auth: vfs unavailable")
	}
	if max <= 0 {
		return nil, errors.New("auth: invalid max")
	}

	var out []byte
	var off uint32
	for len(out) < max {
		want := uint16(max - len(out))
		maxPayload := uint16(kernel.MaxMessageBytes - 11)
		if want > maxPayload {
			want = maxPayload
		}
		b, eof, err := s.vfs.ReadAt(ctx, path, off, want)
		if err != nil {
			return nil, err
		}
		if len(b) == 0 {
			break
		}
		out = append(out, b...)
		off += uint32(len(b))
		if eof {
			break
		}
	}
	if len(out) >= max {
		return nil, errors.New("auth: file too large")
	}
	return out, nil
}

func (s *Service) loadLegacyShadow(ctx *kernel.Context) (userdb.Record, bool, error) {
	if err := s.ensureAuthVFS(ctx); err != nil {
		return userdb.Record{}, false, err
	}

	typ, size, err := s.vfs.Stat(ctx, legacyShadowPath)
	if err != nil || typ != proto.VFSEntryFile {
		return userdb.Record{}, false, nil
	}
	if size == 0 {
		return userdb.Record{}, false, errors.New("auth: empty shadow")
	}
	if size > legacyShadowMaxLen {
		return userdb.Record{}, false, errors.New("auth: shadow too large")
	}

	b, err := s.readFileLimited(ctx, legacyShadowPath, legacyShadowMaxLen)
	if err != nil {
		return userdb.Record{}, false, err
	}
	rec, err := parseLegacyShadow(strings.TrimSpace(string(b)))
	if err != nil {
		return userdb.Record{}, false, err
	}
	return rec, true, nil
}

func parseLegacyShadow(line string) (userdb.Record, error) {
	parts := strings.Split(strings.TrimSpace(line), ":")

	switch len(parts) {
	case 4:
		user := parts[0]
		if user != "root" {
			return userdb.Record{}, errors.New("auth: unknown user")
		}
		iter, err := parseInt(parts[1])
		if err != nil || iter <= 0 || iter > 1_000_000 {
			return userdb.Record{}, errors.New("auth: bad iter")
		}
		saltB, err := hex.DecodeString(parts[2])
		if err != nil || len(saltB) != 16 {
			return userdb.Record{}, errors.New("auth: bad salt")
		}
		hashB, err := hex.DecodeString(parts[3])
		if err != nil || len(hashB) != 32 {
			return userdb.Record{}, errors.New("auth: bad hash")
		}

		var rec userdb.Record
		rec.Name = user
		rec.Role = userdb.RoleAdmin
		rec.Home = "/"
		rec.Scheme = userdb.SchemeLegacySHA256Iter
		rec.Iter = iter
		copy(rec.Salt[:], saltB)
		copy(rec.Hash[:], hashB)
		return rec, nil

	case 5:
		user := parts[0]
		if user != "root" {
			return userdb.Record{}, errors.New("auth: unknown user")
		}
		scheme, ok := userdb.ParsePasswordScheme(parts[1])
		if !ok {
			return userdb.Record{}, errors.New("auth: unknown scheme")
		}
		iter, err := parseInt(parts[2])
		if err != nil || iter <= 0 || iter > 1_000_000 {
			return userdb.Record{}, errors.New("auth: bad iter")
		}
		saltB, err := hex.DecodeString(parts[3])
		if err != nil || len(saltB) != 16 {
			return userdb.Record{}, errors.New("auth: bad salt")
		}
		hashB, err := hex.DecodeString(parts[4])
		if err != nil || len(hashB) != 32 {
			return userdb.Record{}, errors.New("auth: bad hash")
		}

		var rec userdb.Record
		rec.Name = user
		rec.Role = userdb.RoleAdmin
		rec.Home = "/"
		rec.Scheme = scheme
		rec.Iter = iter
		copy(rec.Salt[:], saltB)
		copy(rec.Hash[:], hashB)
		return rec, nil

	default:
		return userdb.Record{}, errors.New("auth: bad shadow format")
	}
}

func parseInt(s string) (int, error) {
	if s == "" {
		return 0, errors.New("empty")
	}
	n := 0
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch < '0' || ch > '9' {
			return 0, errors.New("bad digit")
		}
		n = n*10 + int(ch-'0')
		if n > 1_000_000 {
			return 0, errors.New("too big")
		}
	}
	return n, nil
}
