package userdb

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
)

const (
	UsersPath = "/etc/users"

	// MaxFileBytes bounds parsing to avoid allocation bombs.
	MaxFileBytes = 4096

	MaxUsers   = 32
	MaxNameLen = 32
	MaxHomeLen = 128
)

type Role uint8

const (
	RoleUnknown Role = iota
	RoleUser
	RoleAdmin
)

func ParseRole(s string) (Role, bool) {
	switch s {
	case "user":
		return RoleUser, true
	case "admin":
		return RoleAdmin, true
	default:
		return RoleUnknown, false
	}
}

func (r Role) String() string {
	switch r {
	case RoleUser:
		return "user"
	case RoleAdmin:
		return "admin"
	default:
		return "unknown"
	}
}

type PasswordScheme uint8

const (
	SchemeUnknown PasswordScheme = iota
	SchemeLegacySHA256Iter
	SchemePBKDF2SHA256
)

func ParsePasswordScheme(s string) (PasswordScheme, bool) {
	switch s {
	case "pbkdf2-sha256":
		return SchemePBKDF2SHA256, true
	case "sha256-iter":
		return SchemeLegacySHA256Iter, true
	default:
		return SchemeUnknown, false
	}
}

func (s PasswordScheme) String() string {
	switch s {
	case SchemePBKDF2SHA256:
		return "pbkdf2-sha256"
	case SchemeLegacySHA256Iter:
		return "sha256-iter"
	default:
		return "unknown"
	}
}

type Record struct {
	Name   string
	Role   Role
	Home   string
	Scheme PasswordScheme
	Iter   int
	Salt   [16]byte
	Hash   [32]byte
}

func (r Record) VerifyPassword(pass []byte) bool {
	want := r.Hash

	var got [32]byte
	switch r.Scheme {
	case SchemePBKDF2SHA256:
		got = HashPasswordPBKDF2SHA256(r.Iter, r.Salt, pass)
	case SchemeLegacySHA256Iter:
		got = HashPasswordLegacySHA256Iter(r.Iter, r.Salt, pass)
	default:
		return false
	}
	return subtle.ConstantTimeCompare(want[:], got[:]) == 1
}

func HashPasswordLegacySHA256Iter(iter int, salt [16]byte, pass []byte) [32]byte {
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

func HashPasswordPBKDF2SHA256(iter int, salt [16]byte, pass []byte) [32]byte {
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

func ParseUsersFile(b []byte) ([]Record, error) {
	if len(b) == 0 {
		return nil, errors.New("empty")
	}
	if len(b) > MaxFileBytes {
		return nil, errors.New("too large")
	}

	lines := strings.Split(string(b), "\n")
	out := make([]Record, 0, len(lines))
	seen := make(map[string]struct{}, 8)
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		rec, err := parseUserLine(line)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[rec.Name]; ok {
			return nil, errors.New("duplicate user")
		}
		seen[rec.Name] = struct{}{}
		out = append(out, rec)
		if len(out) > MaxUsers {
			return nil, errors.New("too many users")
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no users")
	}
	return out, nil
}

func FormatUsersFile(users []Record) ([]byte, error) {
	if len(users) == 0 {
		return nil, errors.New("no users")
	}
	if len(users) > MaxUsers {
		return nil, errors.New("too many users")
	}
	cp := append([]Record(nil), users...)
	sort.Slice(cp, func(i, j int) bool { return cp[i].Name < cp[j].Name })

	seen := make(map[string]struct{}, len(cp))
	var b strings.Builder
	for _, u := range cp {
		if err := validateRecord(u); err != nil {
			return nil, err
		}
		if _, ok := seen[u.Name]; ok {
			return nil, errors.New("duplicate user")
		}
		seen[u.Name] = struct{}{}

		fmt.Fprintf(
			&b,
			"%s:%s:%s:%s:%d:%s:%s\n",
			u.Name,
			u.Role.String(),
			u.Home,
			u.Scheme.String(),
			u.Iter,
			hex.EncodeToString(u.Salt[:]),
			hex.EncodeToString(u.Hash[:]),
		)
	}
	if b.Len() > MaxFileBytes {
		return nil, errors.New("too large")
	}
	return []byte(b.String()), nil
}

func Find(users []Record, name string) (Record, bool) {
	for _, u := range users {
		if u.Name == name {
			return u, true
		}
	}
	return Record{}, false
}

func parseUserLine(line string) (Record, error) {
	parts := strings.Split(line, ":")
	if len(parts) != 7 {
		return Record{}, errors.New("bad record")
	}
	name := parts[0]
	if err := validateUsername(name); err != nil {
		return Record{}, err
	}
	role, ok := ParseRole(parts[1])
	if !ok {
		return Record{}, errors.New("bad role")
	}
	home, err := validateHome(parts[2])
	if err != nil {
		return Record{}, err
	}
	scheme, ok := ParsePasswordScheme(parts[3])
	if !ok {
		return Record{}, errors.New("bad scheme")
	}
	iter, err := parseInt(parts[4])
	if err != nil || iter <= 0 || iter > 1_000_000 {
		return Record{}, errors.New("bad iter")
	}
	saltB, err := hex.DecodeString(parts[5])
	if err != nil || len(saltB) != 16 {
		return Record{}, errors.New("bad salt")
	}
	hashB, err := hex.DecodeString(parts[6])
	if err != nil || len(hashB) != 32 {
		return Record{}, errors.New("bad hash")
	}
	var rec Record
	rec.Name = name
	rec.Role = role
	rec.Home = home
	rec.Scheme = scheme
	rec.Iter = iter
	copy(rec.Salt[:], saltB)
	copy(rec.Hash[:], hashB)
	return rec, nil
}

func validateRecord(rec Record) error {
	if err := validateUsername(rec.Name); err != nil {
		return err
	}
	if rec.Role != RoleUser && rec.Role != RoleAdmin {
		return errors.New("bad role")
	}
	home, err := validateHome(rec.Home)
	if err != nil {
		return err
	}
	if home != rec.Home {
		return errors.New("bad home")
	}
	if rec.Scheme != SchemePBKDF2SHA256 && rec.Scheme != SchemeLegacySHA256Iter {
		return errors.New("bad scheme")
	}
	if rec.Iter <= 0 || rec.Iter > 1_000_000 {
		return errors.New("bad iter")
	}
	return nil
}

func validateUsername(name string) error {
	if name == "" {
		return errors.New("bad username")
	}
	if len(name) > MaxNameLen {
		return errors.New("bad username")
	}
	for i := 0; i < len(name); i++ {
		ch := name[i]
		if ch >= 'a' && ch <= 'z' {
			continue
		}
		if ch >= '0' && ch <= '9' {
			continue
		}
		switch ch {
		case '_', '-':
			continue
		default:
			return errors.New("bad username")
		}
	}
	if name[0] >= '0' && name[0] <= '9' {
		return errors.New("bad username")
	}
	return nil
}

func validateHome(home string) (string, error) {
	home = strings.TrimSpace(home)
	if home == "" {
		return "", errors.New("bad home")
	}
	if len(home) > MaxHomeLen {
		return "", errors.New("bad home")
	}
	if !strings.HasPrefix(home, "/") {
		return "", errors.New("bad home")
	}
	home = path.Clean(home)
	if home == "." {
		home = "/"
	}
	if !strings.HasPrefix(home, "/") {
		home = "/" + home
	}
	return home, nil
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
