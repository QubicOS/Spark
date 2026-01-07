package shell

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	vfsclient "spark/sparkos/client/vfs"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

const (
	shadowPath   = "/etc/shadow"
	shadowMaxLen = 512

	defaultPBKDF2Iters = 20000
)

type shadowScheme uint8

const (
	shadowSchemeLegacySHAIter shadowScheme = iota + 1
	shadowSchemePBKDF2SHA256
)

type shadowRecord struct {
	user   string
	scheme shadowScheme
	iter   int
	salt   [16]byte
	hash   [32]byte
}

func (s *Service) ensureAuthVFS(ctx *kernel.Context) error {
	if s.vfs == nil && s.vfsCap.Valid() {
		s.vfs = vfsclient.New(s.vfsCap)
	}
	if s.vfs == nil {
		return errors.New("auth: vfs unavailable")
	}
	return nil
}

func (s *Service) loadShadow(ctx *kernel.Context) (shadowRecord, bool, error) {
	if err := s.ensureAuthVFS(ctx); err != nil {
		return shadowRecord{}, false, err
	}

	typ, size, err := s.vfs.Stat(ctx, shadowPath)
	if err != nil || typ != proto.VFSEntryFile {
		return shadowRecord{}, false, nil
	}
	if size == 0 {
		return shadowRecord{}, false, errors.New("auth: empty shadow")
	}
	if size > shadowMaxLen {
		return shadowRecord{}, false, errors.New("auth: shadow too large")
	}

	b, err := s.readFileLimited(ctx, shadowPath, shadowMaxLen)
	if err != nil {
		return shadowRecord{}, false, err
	}
	rec, err := parseShadow(string(b))
	if err != nil {
		return shadowRecord{}, false, err
	}
	return rec, true, nil
}

func (s *Service) writeShadow(ctx *kernel.Context, rec shadowRecord) error {
	if err := s.ensureAuthVFS(ctx); err != nil {
		return err
	}
	if err := s.vfs.Mkdir(ctx, "/etc"); err != nil {
		_ = err
	}

	line, err := formatShadow(rec)
	if err != nil {
		return err
	}
	_, err = s.vfs.Write(ctx, shadowPath, proto.VFSWriteTruncate, []byte(line))
	if err != nil {
		return err
	}
	return nil
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

func parseShadow(s string) (shadowRecord, error) {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, ":")

	var rec shadowRecord
	switch len(parts) {
	case 4:
		user := parts[0]
		if user != "root" {
			return shadowRecord{}, errors.New("auth: unknown user")
		}
		iter, err := parseInt(parts[1])
		if err != nil || iter <= 0 || iter > 1_000_000 {
			return shadowRecord{}, errors.New("auth: bad iter")
		}
		saltB, err := hex.DecodeString(parts[2])
		if err != nil || len(saltB) != 16 {
			return shadowRecord{}, errors.New("auth: bad salt")
		}
		hashB, err := hex.DecodeString(parts[3])
		if err != nil || len(hashB) != 32 {
			return shadowRecord{}, errors.New("auth: bad hash")
		}
		rec.user = user
		rec.scheme = shadowSchemeLegacySHAIter
		rec.iter = iter
		copy(rec.salt[:], saltB)
		copy(rec.hash[:], hashB)
		return rec, nil

	case 5:
		user := parts[0]
		if user != "root" {
			return shadowRecord{}, errors.New("auth: unknown user")
		}
		scheme, ok := parseShadowScheme(parts[1])
		if !ok {
			return shadowRecord{}, errors.New("auth: unknown scheme")
		}
		iter, err := parseInt(parts[2])
		if err != nil || iter <= 0 || iter > 1_000_000 {
			return shadowRecord{}, errors.New("auth: bad iter")
		}
		saltB, err := hex.DecodeString(parts[3])
		if err != nil || len(saltB) != 16 {
			return shadowRecord{}, errors.New("auth: bad salt")
		}
		hashB, err := hex.DecodeString(parts[4])
		if err != nil || len(hashB) != 32 {
			return shadowRecord{}, errors.New("auth: bad hash")
		}
		rec.user = user
		rec.scheme = scheme
		rec.iter = iter
		copy(rec.salt[:], saltB)
		copy(rec.hash[:], hashB)
		return rec, nil

	default:
		return shadowRecord{}, errors.New("auth: bad shadow format")
	}
}

func formatShadow(rec shadowRecord) (string, error) {
	if rec.user != "root" {
		return "", errors.New("auth: only root supported")
	}
	if rec.scheme == 0 {
		return "", errors.New("auth: invalid scheme")
	}
	if rec.iter <= 0 {
		return "", errors.New("auth: invalid iter")
	}
	return fmt.Sprintf(
		"%s:%s:%d:%s:%s\n",
		rec.user,
		rec.scheme.String(),
		rec.iter,
		hex.EncodeToString(rec.salt[:]),
		hex.EncodeToString(rec.hash[:]),
	), nil
}

func parseShadowScheme(s string) (shadowScheme, bool) {
	switch s {
	case "pbkdf2-sha256":
		return shadowSchemePBKDF2SHA256, true
	case "sha256-iter":
		return shadowSchemeLegacySHAIter, true
	default:
		return 0, false
	}
}

func (s shadowScheme) String() string {
	switch s {
	case shadowSchemePBKDF2SHA256:
		return "pbkdf2-sha256"
	case shadowSchemeLegacySHAIter:
		return "sha256-iter"
	default:
		return "unknown"
	}
}

func hashPasswordLegacySHA256Iter(iter int, salt [16]byte, pass []byte) [32]byte {
	h := sha256.New()
	_, _ = h.Write(salt[:])
	_, _ = h.Write(pass)
	var sum [32]byte
	_ = h.Sum(sum[:0])
	for i := 1; i < iter; i++ {
		sum = sha256.Sum256(sum[:])
	}
	return sum
}

func hashPasswordPBKDF2SHA256(iter int, salt [16]byte, pass []byte) [32]byte {
	if iter <= 0 {
		iter = 1
	}
	var block [4]byte
	binary.BigEndian.PutUint32(block[:], 1)

	var u [32]byte
	var t [32]byte
	mac := hmac.New(sha256.New, pass)
	_, _ = mac.Write(salt[:])
	_, _ = mac.Write(block[:])
	_ = mac.Sum(u[:0])
	t = u

	for i := 1; i < iter; i++ {
		mac.Reset()
		_, _ = mac.Write(u[:])
		_ = mac.Sum(u[:0])
		for j := range t {
			t[j] ^= u[j]
		}
	}
	return t
}

func verifyPassword(rec shadowRecord, pass []byte) bool {
	want := rec.hash
	var got [32]byte
	switch rec.scheme {
	case shadowSchemePBKDF2SHA256:
		got = hashPasswordPBKDF2SHA256(rec.iter, rec.salt, pass)
	case shadowSchemeLegacySHAIter:
		got = hashPasswordLegacySHA256Iter(rec.iter, rec.salt, pass)
	default:
		return false
	}
	return subtle.ConstantTimeCompare(want[:], got[:]) == 1
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
