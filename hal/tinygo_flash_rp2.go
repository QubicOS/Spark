//go:build tinygo && baremetal && (rp2040 || rp2350)

package hal

import (
	"fmt"
	"machine"
)

type rp2Flash struct{}

func newRP2Flash() Flash {
	return rp2Flash{}
}

func (rp2Flash) SizeBytes() uint32 {
	sz := machine.Flash.Size()
	if sz <= 0 {
		return 0
	}
	if sz > int64(^uint32(0)) {
		return ^uint32(0)
	}
	return uint32(sz)
}

func (rp2Flash) EraseBlockBytes() uint32 {
	bs := machine.Flash.EraseBlockSize()
	if bs <= 0 {
		return 0
	}
	if bs > int64(^uint32(0)) {
		return ^uint32(0)
	}
	return uint32(bs)
}

func (rp2Flash) ReadAt(p []byte, off uint32) (int, error) {
	n, err := machine.Flash.ReadAt(p, int64(off))
	if err != nil {
		return n, fmt.Errorf("flash read at %d: %w", off, err)
	}
	return n, nil
}

func (rp2Flash) WriteAt(p []byte, off uint32) (int, error) {
	n, err := machine.Flash.WriteAt(p, int64(off))
	if err != nil {
		return n, fmt.Errorf("flash write at %d: %w", off, err)
	}
	return n, nil
}

func (rp2Flash) Erase(off, size uint32) error {
	if size == 0 {
		return nil
	}
	bs := rp2Flash{}.EraseBlockBytes()
	if bs == 0 {
		return ErrNotImplemented
	}
	if off%bs != 0 || size%bs != 0 {
		return fmt.Errorf("flash erase off=%d size=%d: %w", off, size, ErrNotImplemented)
	}
	return machine.Flash.EraseBlocks(int64(off/bs), int64(size/bs))
}
