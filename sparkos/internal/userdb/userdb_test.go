package userdb

import (
	"bytes"
	"testing"
)

func TestUsersFileRoundTrip(t *testing.T) {
	var salt [16]byte
	for i := range salt {
		salt[i] = byte(i)
	}
	pass := []byte("hunter2")

	root := Record{
		Name:   "root",
		Role:   RoleAdmin,
		Home:   "/",
		Scheme: SchemePBKDF2SHA256,
		Iter:   2,
		Salt:   salt,
		Hash:   HashPasswordPBKDF2SHA256(2, salt, pass),
	}

	alice := Record{
		Name:   "alice",
		Role:   RoleUser,
		Home:   "/home/alice",
		Scheme: SchemeLegacySHA256Iter,
		Iter:   3,
		Salt:   salt,
		Hash:   HashPasswordLegacySHA256Iter(3, salt, []byte("pw")),
	}

	b, err := FormatUsersFile([]Record{root, alice})
	if err != nil {
		t.Fatalf("FormatUsersFile: %v", err)
	}
	users, err := ParseUsersFile(b)
	if err != nil {
		t.Fatalf("ParseUsersFile: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("len(users)=%d, want 2", len(users))
	}

	gotRoot, ok := Find(users, "root")
	if !ok {
		t.Fatalf("missing root")
	}
	if gotRoot.Role != RoleAdmin || gotRoot.Home != "/" || gotRoot.Scheme != SchemePBKDF2SHA256 || gotRoot.Iter != 2 {
		t.Fatalf("root mismatch: %+v", gotRoot)
	}
	if !gotRoot.VerifyPassword(pass) {
		t.Fatalf("root password verify failed")
	}

	gotAlice, ok := Find(users, "alice")
	if !ok {
		t.Fatalf("missing alice")
	}
	if gotAlice.Role != RoleUser || gotAlice.Home != "/home/alice" {
		t.Fatalf("alice mismatch: %+v", gotAlice)
	}
	if !gotAlice.VerifyPassword([]byte("pw")) {
		t.Fatalf("alice password verify failed")
	}
}

func TestParseUsersFileRejectsDuplicate(t *testing.T) {
	in := []byte("root:admin:/:pbkdf2-sha256:1:00000000000000000000000000000000:0000000000000000000000000000000000000000000000000000000000000000\n" +
		"root:admin:/:pbkdf2-sha256:1:00000000000000000000000000000000:0000000000000000000000000000000000000000000000000000000000000000\n")
	if _, err := ParseUsersFile(in); err == nil {
		t.Fatalf("ParseUsersFile: expected error")
	}
}

func TestFormatUsersFileRejectsInvalidRecord(t *testing.T) {
	var salt [16]byte
	var hash [32]byte
	_, err := FormatUsersFile([]Record{{
		Name:   "Bad!",
		Role:   RoleUser,
		Home:   "/home/bad",
		Scheme: SchemePBKDF2SHA256,
		Iter:   1,
		Salt:   salt,
		Hash:   hash,
	}})
	if err == nil {
		t.Fatalf("FormatUsersFile: expected error")
	}
}

func TestFormatUsersFileIsDeterministic(t *testing.T) {
	var salt [16]byte
	var hash [32]byte
	a := Record{Name: "b", Role: RoleUser, Home: "/home/b", Scheme: SchemePBKDF2SHA256, Iter: 1, Salt: salt, Hash: hash}
	b := Record{Name: "a", Role: RoleUser, Home: "/home/a", Scheme: SchemePBKDF2SHA256, Iter: 1, Salt: salt, Hash: hash}

	out1, err := FormatUsersFile([]Record{a, b})
	if err != nil {
		t.Fatalf("FormatUsersFile(1): %v", err)
	}
	out2, err := FormatUsersFile([]Record{b, a})
	if err != nil {
		t.Fatalf("FormatUsersFile(2): %v", err)
	}
	if !bytes.Equal(out1, out2) {
		t.Fatalf("FormatUsersFile not deterministic")
	}
}
