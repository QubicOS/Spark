package vfs

import (
	"testing"

	"spark/sparkos/fs/littlefs"
)

type dummyFS struct{}

func (dummyFS) ListDir(string, func(name string, info littlefs.Info) bool) error { return nil }
func (dummyFS) Mkdir(string) error                                               { return nil }
func (dummyFS) Remove(string) error                                              { return nil }
func (dummyFS) Rename(string, string) error                                      { return nil }
func (dummyFS) Stat(string) (littlefs.Info, error)                               { return littlefs.Info{}, nil }
func (dummyFS) ReadAt(string, []byte, uint32) (int, bool, error)                 { return 0, true, nil }
func (dummyFS) OpenWriter(string, littlefs.WriteMode) (writeHandle, error)       { return nil, nil }

func TestResolve_SDWithoutFlash(t *testing.T) {
	s := &Service{sd: dummyFS{}}

	fs, rel, ok := s.resolve("/sd")
	if !ok || fs == nil || rel != "/" {
		t.Fatalf("resolve(/sd) = ok=%v fs=%v rel=%q; want ok=true rel=/", ok, fs, rel)
	}

	fs, rel, ok = s.resolve("/sd/hello.txt")
	if !ok || fs == nil || rel != "/hello.txt" {
		t.Fatalf("resolve(/sd/hello.txt) = ok=%v fs=%v rel=%q; want ok=true rel=/hello.txt", ok, fs, rel)
	}

	if _, _, ok := s.resolve("/"); ok {
		t.Fatalf("resolve(/) ok=true; want ok=false when flash fs is nil")
	}
	if _, _, ok := s.resolve("/etc"); ok {
		t.Fatalf("resolve(/etc) ok=true; want ok=false when flash fs is nil")
	}
}
