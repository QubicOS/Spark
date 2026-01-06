//go:build !tinygo

package hal

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
)

const (
	hostFlashDefaultPath      = "spark.flash"
	hostFlashDefaultSizeBytes = 2 * 1024 * 1024
	hostFlashEraseBlockBytes  = 4096
)

var ErrFlashWriteRequiresErase = errors.New("flash write requires erase")

type hostFlash struct {
	mu       sync.Mutex
	f        *os.File
	size     uint32
	scratch4 [hostFlashEraseBlockBytes]byte
}

func newHostFlash() *hostFlash {
	path := os.Getenv("SPARK_FLASH_PATH")
	if path == "" {
		path = hostFlashDefaultPath
	}

	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return &hostFlash{f: nil}
	}

	size := uint32(hostFlashDefaultSizeBytes)
	if st, err := f.Stat(); err == nil && st.Size() > 0 {
		if st.Size() > int64(^uint32(0)) {
			_ = f.Close()
			return &hostFlash{f: nil}
		}
		size = uint32(st.Size())
	} else {
		if err := f.Truncate(int64(size)); err != nil {
			_ = f.Close()
			return &hostFlash{f: nil}
		}
	}

	hf := &hostFlash{f: f, size: size}
	for i := range hf.scratch4 {
		hf.scratch4[i] = 0xFF
	}
	return hf
}

func (f *hostFlash) SizeBytes() uint32 { return f.size }
func (f *hostFlash) EraseBlockBytes() uint32 {
	return hostFlashEraseBlockBytes
}

func (f *hostFlash) ReadAt(p []byte, off uint32) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.f == nil {
		return 0, ErrNotImplemented
	}
	if off >= f.size {
		return 0, fmt.Errorf("flash read at %d: %w", off, os.ErrInvalid)
	}
	maxN := int(f.size - off)
	if len(p) > maxN {
		p = p[:maxN]
	}
	return f.f.ReadAt(p, int64(off))
}

func (f *hostFlash) WriteAt(p []byte, off uint32) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.f == nil {
		return 0, ErrNotImplemented
	}
	if off >= f.size {
		return 0, fmt.Errorf("flash write at %d: %w", off, os.ErrInvalid)
	}
	maxN := int(f.size - off)
	if len(p) > maxN {
		p = p[:maxN]
	}

	buf := make([]byte, len(p))
	if _, err := f.f.ReadAt(buf, int64(off)); err != nil && !errors.Is(err, io.EOF) {
		return 0, fmt.Errorf("flash read before write at %d: %w", off, err)
	}
	for i := range p {
		if buf[i]&p[i] != p[i] {
			return 0, ErrFlashWriteRequiresErase
		}
	}
	return f.f.WriteAt(p, int64(off))
}

func (f *hostFlash) Erase(off, size uint32) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.f == nil {
		return ErrNotImplemented
	}
	if size == 0 {
		return nil
	}
	if off%hostFlashEraseBlockBytes != 0 || size%hostFlashEraseBlockBytes != 0 {
		return fmt.Errorf("flash erase off=%d size=%d: %w", off, size, os.ErrInvalid)
	}
	if off >= f.size || off+size > f.size {
		return fmt.Errorf("flash erase off=%d size=%d: %w", off, size, os.ErrInvalid)
	}

	for size > 0 {
		if _, err := f.f.WriteAt(f.scratch4[:], int64(off)); err != nil {
			return fmt.Errorf("flash erase block at %d: %w", off, err)
		}
		off += hostFlashEraseBlockBytes
		size -= hostFlashEraseBlockBytes
	}
	return nil
}
