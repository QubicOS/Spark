//go:build !tinygo

package littlefs

/*
#cgo CFLAGS: -std=c99 -O2 -Wall -Wextra

#include <stdint.h>
#include <stdlib.h>

#include "lfs.h"

void spark_lfs_config_init(struct lfs_config *cfg);
*/
import "C"

import (
	"errors"
	"fmt"
	"runtime/cgo"
	"sync"
	"unsafe"
)

var (
	// ErrNotMounted indicates that the filesystem is not mounted.
	ErrNotMounted = errors.New("littlefs: not mounted")
	// ErrNotFound indicates that a path does not exist.
	ErrNotFound = errors.New("littlefs: not found")
	// ErrExists indicates that a path already exists.
	ErrExists = errors.New("littlefs: already exists")
	// ErrNotDir indicates that a path is not a directory.
	ErrNotDir = errors.New("littlefs: not a directory")
	// ErrIsDir indicates that a path is a directory when a file was expected.
	ErrIsDir = errors.New("littlefs: is a directory")
	// ErrNoSpace indicates that the filesystem is out of space.
	ErrNoSpace = errors.New("littlefs: no space")
	// ErrInvalid indicates invalid arguments or an invalid filesystem state.
	ErrInvalid = errors.New("littlefs: invalid")
	// ErrCorrupt indicates corrupted on-flash metadata.
	ErrCorrupt = errors.New("littlefs: corrupt")
)

type Flash interface {
	SizeBytes() uint32
	EraseBlockBytes() uint32
	ReadAt(p []byte, off uint32) (int, error)
	WriteAt(p []byte, off uint32) (int, error)
	Erase(off, size uint32) error
}

// Options configures a LittleFS mount.
type Options struct {
	ReadSize      uint32
	ProgSize      uint32
	CacheSize     uint32
	LookaheadSize uint32
	BlockCycles   int32
}

// WriteMode controls how a file is created/updated.
type WriteMode uint8

const (
	// WriteTruncate truncates the file before writing.
	WriteTruncate WriteMode = iota
	// WriteAppend appends to an existing file (creating it if needed).
	WriteAppend
)

// FS is a mounted LittleFS instance.
type FS struct {
	mu sync.Mutex

	flash Flash

	lfs C.lfs_t
	cfg C.struct_lfs_config

	handle cgo.Handle

	mounted bool
}

// New prepares a LittleFS instance on top of flash.
func New(flash Flash, opts Options) (*FS, error) {
	if flash == nil {
		return nil, errors.New("littlefs: nil flash")
	}

	blockSize := flash.EraseBlockBytes()
	if blockSize == 0 {
		return nil, errors.New("littlefs: erase block size is zero")
	}
	if flash.SizeBytes()%blockSize != 0 {
		return nil, fmt.Errorf("littlefs: flash size not multiple of erase block size: %d %% %d", flash.SizeBytes(), blockSize)
	}

	if opts.ReadSize == 0 {
		opts.ReadSize = 256
	}
	if opts.ProgSize == 0 {
		opts.ProgSize = 256
	}
	if opts.CacheSize == 0 {
		opts.CacheSize = 256
	}
	if opts.LookaheadSize == 0 {
		blocks := flash.SizeBytes() / blockSize
		need := (blocks + 7) / 8
		opts.LookaheadSize = need
		if opts.LookaheadSize < 64 {
			opts.LookaheadSize = 64
		}
	}
	if opts.BlockCycles == 0 {
		opts.BlockCycles = 500
	}

	fs := &FS{flash: flash}
	fs.handle = cgo.NewHandle(fs)

	C.spark_lfs_config_init(&fs.cfg)
	fs.cfg.context = unsafe.Pointer(uintptr(fs.handle))
	fs.cfg.read_size = C.lfs_size_t(opts.ReadSize)
	fs.cfg.prog_size = C.lfs_size_t(opts.ProgSize)
	fs.cfg.block_size = C.lfs_size_t(blockSize)
	fs.cfg.block_count = C.lfs_size_t(flash.SizeBytes() / blockSize)
	fs.cfg.cache_size = C.lfs_size_t(opts.CacheSize)
	fs.cfg.lookahead_size = C.lfs_size_t(opts.LookaheadSize)
	fs.cfg.block_cycles = C.int32_t(opts.BlockCycles)

	return fs, nil
}

// Close releases internal resources.
func (fs *FS) Close() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if fs.mounted {
		_ = fs.unmountLocked()
	}
	fs.handle.Delete()
	return nil
}

// Format initializes a fresh filesystem on flash.
func (fs *FS) Format() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if fs.mounted {
		return errors.New("littlefs: already mounted")
	}

	rc := C.lfs_format(&fs.lfs, &fs.cfg)
	if rc != 0 {
		return fmt.Errorf("littlefs format: %w", decodeErr(int(rc)))
	}
	return nil
}

// Mount mounts an existing filesystem from flash.
func (fs *FS) Mount() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if fs.mounted {
		return nil
	}

	rc := C.lfs_mount(&fs.lfs, &fs.cfg)
	if rc != 0 {
		return fmt.Errorf("littlefs mount: %w", decodeErr(int(rc)))
	}
	fs.mounted = true
	return nil
}

// MountOrFormat attempts to mount and formats if the filesystem appears missing/corrupt.
func (fs *FS) MountOrFormat() error {
	if err := fs.Mount(); err == nil {
		return nil
	} else if errors.Is(err, ErrCorrupt) || errors.Is(err, ErrInvalid) {
		if err := fs.Format(); err != nil {
			return err
		}
		return fs.Mount()
	} else {
		return err
	}
}

// Unmount unmounts the filesystem.
func (fs *FS) Unmount() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.unmountLocked()
}

func (fs *FS) unmountLocked() error {
	if !fs.mounted {
		return nil
	}
	rc := C.lfs_unmount(&fs.lfs)
	if rc != 0 {
		return fmt.Errorf("littlefs unmount: %w", decodeErr(int(rc)))
	}
	fs.mounted = false
	return nil
}

// Mkdir creates a directory.
func (fs *FS) Mkdir(path string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if err := fs.ensureMountedLocked(); err != nil {
		return err
	}
	cpath, freeFn, err := cString(path)
	if err != nil {
		return err
	}
	defer freeFn()

	rc := C.lfs_mkdir(&fs.lfs, cpath)
	if rc != 0 {
		return fmt.Errorf("littlefs mkdir %q: %w", path, decodeErr(int(rc)))
	}
	return nil
}

// Info describes a path.
type Info struct {
	Type VFSType
	Size uint32
}

// VFSType is the file type returned by Stat.
type VFSType uint8

const (
	TypeUnknown VFSType = iota
	TypeFile
	TypeDir
)

// Stat returns path information.
func (fs *FS) Stat(path string) (Info, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if err := fs.ensureMountedLocked(); err != nil {
		return Info{}, err
	}
	cpath, freeFn, err := cString(path)
	if err != nil {
		return Info{}, err
	}
	defer freeFn()

	var info C.struct_lfs_info
	rc := C.lfs_stat(&fs.lfs, cpath, &info)
	if rc != 0 {
		return Info{}, fmt.Errorf("littlefs stat %q: %w", path, decodeErr(int(rc)))
	}

	return Info{Type: decodeType(info._type), Size: uint32(info.size)}, nil
}

// ListDir iterates directory entries, stopping when fn returns false.
func (fs *FS) ListDir(path string, fn func(name string, info Info) bool) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if err := fs.ensureMountedLocked(); err != nil {
		return err
	}
	cpath, freeFn, err := cString(path)
	if err != nil {
		return err
	}
	defer freeFn()

	var dir C.lfs_dir_t
	rc := C.lfs_dir_open(&fs.lfs, &dir, cpath)
	if rc != 0 {
		return fmt.Errorf("littlefs dir open %q: %w", path, decodeErr(int(rc)))
	}
	defer func() { _ = C.lfs_dir_close(&fs.lfs, &dir) }()

	for {
		var cinfo C.struct_lfs_info
		rc := C.lfs_dir_read(&fs.lfs, &dir, &cinfo)
		if rc < 0 {
			return fmt.Errorf("littlefs dir read %q: %w", path, decodeErr(int(rc)))
		}
		if rc == 0 {
			return nil
		}

		name := C.GoString((*C.char)(unsafe.Pointer(&cinfo.name[0])))
		if name == "." || name == ".." {
			continue
		}
		inf := Info{Type: decodeType(cinfo._type), Size: uint32(cinfo.size)}
		if !fn(name, inf) {
			return nil
		}
	}
}

// ReadAt reads up to len(p) bytes from file at off.
func (fs *FS) ReadAt(path string, p []byte, off uint32) (n int, eof bool, err error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if err := fs.ensureMountedLocked(); err != nil {
		return 0, false, err
	}
	cpath, freeFn, err := cString(path)
	if err != nil {
		return 0, false, err
	}
	defer freeFn()

	var f C.lfs_file_t
	rc := C.lfs_file_open(&fs.lfs, &f, cpath, C.LFS_O_RDONLY)
	if rc != 0 {
		return 0, false, fmt.Errorf("littlefs open %q: %w", path, decodeErr(int(rc)))
	}
	defer func() { _ = C.lfs_file_close(&fs.lfs, &f) }()

	if rc := C.lfs_file_seek(&fs.lfs, &f, C.lfs_soff_t(off), C.LFS_SEEK_SET); rc < 0 {
		return 0, false, fmt.Errorf("littlefs seek %q off=%d: %w", path, off, decodeErr(int(rc)))
	}

	if len(p) == 0 {
		return 0, false, nil
	}

	rc = C.lfs_file_read(&fs.lfs, &f, unsafe.Pointer(unsafe.SliceData(p)), C.lfs_size_t(len(p)))
	if rc < 0 {
		return 0, false, fmt.Errorf("littlefs read %q off=%d: %w", path, off, decodeErr(int(rc)))
	}

	n = int(rc)
	eof = n < len(p)
	return n, eof, nil
}

// Writer is an incremental file writer.
type Writer struct {
	fs      *FS
	path    string
	file    C.lfs_file_t
	written uint32
	closed  bool
}

// OpenWriter opens a file for incremental writes.
func (fs *FS) OpenWriter(path string, mode WriteMode) (*Writer, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if err := fs.ensureMountedLocked(); err != nil {
		return nil, err
	}
	cpath, freeFn, err := cString(path)
	if err != nil {
		return nil, err
	}
	defer freeFn()

	flags := C.int(C.LFS_O_WRONLY) | C.int(C.LFS_O_CREAT)
	switch mode {
	case WriteTruncate:
		flags |= C.int(C.LFS_O_TRUNC)
	case WriteAppend:
		flags |= C.int(C.LFS_O_APPEND)
	default:
		return nil, fmt.Errorf("littlefs open writer %q: invalid mode %d", path, mode)
	}

	var f C.lfs_file_t
	rc := C.lfs_file_open(&fs.lfs, &f, cpath, C.int(flags))
	if rc != 0 {
		return nil, fmt.Errorf("littlefs open %q: %w", path, decodeErr(int(rc)))
	}

	return &Writer{fs: fs, path: path, file: f}, nil
}

// Write appends bytes to the open file.
func (w *Writer) Write(p []byte) (int, error) {
	if w.closed {
		return 0, errors.New("littlefs: write on closed writer")
	}
	if len(p) == 0 {
		return 0, nil
	}

	w.fs.mu.Lock()
	defer w.fs.mu.Unlock()

	rc := C.lfs_file_write(&w.fs.lfs, &w.file, unsafe.Pointer(unsafe.SliceData(p)), C.lfs_size_t(len(p)))
	if rc < 0 {
		return 0, fmt.Errorf("littlefs write %q: %w", w.path, decodeErr(int(rc)))
	}
	w.written += uint32(rc)
	return int(rc), nil
}

// Close flushes and closes the file.
func (w *Writer) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true

	w.fs.mu.Lock()
	defer w.fs.mu.Unlock()

	if err := w.fs.ensureMountedLocked(); err != nil {
		_ = C.lfs_file_close(&w.fs.lfs, &w.file)
		return err
	}

	if rc := C.lfs_file_sync(&w.fs.lfs, &w.file); rc < 0 {
		_ = C.lfs_file_close(&w.fs.lfs, &w.file)
		return fmt.Errorf("littlefs sync %q: %w", w.path, decodeErr(int(rc)))
	}
	if rc := C.lfs_file_close(&w.fs.lfs, &w.file); rc != 0 {
		return fmt.Errorf("littlefs close %q: %w", w.path, decodeErr(int(rc)))
	}
	return nil
}

func (w *Writer) BytesWritten() uint32 { return w.written }

func (fs *FS) ensureMountedLocked() error {
	if fs.mounted {
		return nil
	}
	return ErrNotMounted
}

func decodeType(t C.uint8_t) VFSType {
	switch t {
	case C.LFS_TYPE_REG:
		return TypeFile
	case C.LFS_TYPE_DIR:
		return TypeDir
	default:
		return TypeUnknown
	}
}

func decodeErr(rc int) error {
	switch rc {
	case int(C.LFS_ERR_CORRUPT):
		return ErrCorrupt
	case int(C.LFS_ERR_NOENT):
		return ErrNotFound
	case int(C.LFS_ERR_EXIST):
		return ErrExists
	case int(C.LFS_ERR_NOTDIR):
		return ErrNotDir
	case int(C.LFS_ERR_ISDIR):
		return ErrIsDir
	case int(C.LFS_ERR_NOSPC):
		return ErrNoSpace
	case int(C.LFS_ERR_INVAL):
		return ErrInvalid
	case int(C.LFS_ERR_OK):
		return nil
	default:
		if rc == 0 {
			return nil
		}
		return fmt.Errorf("littlefs rc=%d", rc)
	}
}

func cString(s string) (*C.char, func(), error) {
	if len(s) == 0 {
		s = "/"
	}
	cs := C.CString(s)
	if cs == nil {
		return nil, nil, errors.New("littlefs: CString failed")
	}
	return cs, func() { C.free(unsafe.Pointer(cs)) }, nil
}
