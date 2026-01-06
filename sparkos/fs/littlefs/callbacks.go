//go:build !tinygo

package littlefs

/*
#include <stdint.h>
#include "lfs.h"

typedef struct spark_lfs_ctx {
	uintptr_t handle;
} spark_lfs_ctx_t;
*/
import "C"

import (
	"errors"
	"fmt"
	"runtime/cgo"
	"unsafe"
)

//export go_lfs_read
func go_lfs_read(ctx unsafe.Pointer, block C.lfs_block_t, off C.lfs_off_t, buffer unsafe.Pointer, size C.lfs_size_t) C.int {
	fs, err := handleToFS(ctx)
	if err != nil {
		return C.int(C.LFS_ERR_IO)
	}
	return fs.flashRead(block, off, buffer, size)
}

//export go_lfs_prog
func go_lfs_prog(ctx unsafe.Pointer, block C.lfs_block_t, off C.lfs_off_t, buffer unsafe.Pointer, size C.lfs_size_t) C.int {
	fs, err := handleToFS(ctx)
	if err != nil {
		return C.int(C.LFS_ERR_IO)
	}
	return fs.flashProg(block, off, buffer, size)
}

//export go_lfs_erase
func go_lfs_erase(ctx unsafe.Pointer, block C.lfs_block_t) C.int {
	fs, err := handleToFS(ctx)
	if err != nil {
		return C.int(C.LFS_ERR_IO)
	}
	return fs.flashErase(block)
}

//export go_lfs_sync
func go_lfs_sync(ctx unsafe.Pointer) C.int {
	_, err := handleToFS(ctx)
	if err != nil {
		return C.int(C.LFS_ERR_IO)
	}
	return 0
}

func handleToFS(ctx unsafe.Pointer) (*FS, error) {
	if ctx == nil {
		return nil, errors.New("nil context")
	}

	cctx := (*C.spark_lfs_ctx_t)(ctx)
	h := cgo.Handle(uintptr(cctx.handle))
	v := h.Value()
	fs, ok := v.(*FS)
	if !ok || fs == nil {
		return nil, fmt.Errorf("unexpected context type %T", v)
	}
	return fs, nil
}

func (fs *FS) flashRead(block C.lfs_block_t, off C.lfs_off_t, buffer unsafe.Pointer, size C.lfs_size_t) C.int {
	if size == 0 {
		return 0
	}

	dst := unsafe.Slice((*byte)(buffer), int(size))
	addr, ok := fs.blockAddr(uint32(block), uint32(off), uint32(size))
	if !ok {
		return C.int(C.LFS_ERR_INVAL)
	}

	n, err := fs.flash.ReadAt(dst, addr)
	if err != nil || n != len(dst) {
		return C.int(C.LFS_ERR_IO)
	}
	return 0
}

func (fs *FS) flashProg(block C.lfs_block_t, off C.lfs_off_t, buffer unsafe.Pointer, size C.lfs_size_t) C.int {
	if size == 0 {
		return 0
	}

	src := unsafe.Slice((*byte)(buffer), int(size))
	addr, ok := fs.blockAddr(uint32(block), uint32(off), uint32(size))
	if !ok {
		return C.int(C.LFS_ERR_INVAL)
	}

	n, err := fs.flash.WriteAt(src, addr)
	if err != nil || n != len(src) {
		return C.int(C.LFS_ERR_IO)
	}
	return 0
}

func (fs *FS) flashErase(block C.lfs_block_t) C.int {
	addr := uint64(uint32(block)) * uint64(fs.flash.EraseBlockBytes())
	if addr > uint64(^uint32(0)) {
		return C.int(C.LFS_ERR_INVAL)
	}
	if err := fs.flash.Erase(uint32(addr), fs.flash.EraseBlockBytes()); err != nil {
		return C.int(C.LFS_ERR_IO)
	}
	return 0
}

func (fs *FS) blockAddr(block uint32, off uint32, size uint32) (uint32, bool) {
	blockSize := fs.flash.EraseBlockBytes()
	if blockSize == 0 {
		return 0, false
	}
	base := uint64(block) * uint64(blockSize)
	addr := base + uint64(off)
	end := addr + uint64(size)
	if addr > uint64(^uint32(0)) || end > uint64(^uint32(0)) {
		return 0, false
	}
	if end > uint64(fs.flash.SizeBytes()) {
		return 0, false
	}
	return uint32(addr), true
}
