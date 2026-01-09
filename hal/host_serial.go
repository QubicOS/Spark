//go:build !tinygo

package hal

import (
	"os"
	"sync"
)

type hostSerial struct {
	mu sync.Mutex
	r  *os.File
	w  *os.File
}

func (s *hostSerial) Read(p []byte) (int, error) {
	if s.r == nil {
		return 0, ErrNotImplemented
	}
	return s.r.Read(p)
}

func (s *hostSerial) Write(p []byte) (int, error) {
	if s.w == nil {
		return 0, ErrNotImplemented
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}
