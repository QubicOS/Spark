//go:build tinygo

package littlefs

import (
	"errors"
	"fmt"
	"io"
	"os"

	"tinygo.org/x/tinyfs"
	tlfs "tinygo.org/x/tinyfs/littlefs"
)

var (
	ErrNotMounted = errors.New("littlefs: not mounted")
	ErrNotFound   = errors.New("littlefs: not found")
	ErrExists     = errors.New("littlefs: already exists")
	ErrNotDir     = errors.New("littlefs: not a directory")
	ErrIsDir      = errors.New("littlefs: is a directory")
	ErrNoSpace    = errors.New("littlefs: no space")
	ErrInvalid    = errors.New("littlefs: invalid")
	ErrCorrupt    = errors.New("littlefs: corrupt")
	ErrNotEmpty   = errors.New("littlefs: directory not empty")
)

type Flash interface {
	SizeBytes() uint32
	EraseBlockBytes() uint32
	ReadAt(p []byte, off uint32) (int, error)
	WriteAt(p []byte, off uint32) (int, error)
	Erase(off, size uint32) error
}

type Options struct {
	ReadSize      uint32
	ProgSize      uint32
	CacheSize     uint32
	LookaheadSize uint32
	BlockCycles   int32
}

type WriteMode uint8

const (
	WriteTruncate WriteMode = iota
	WriteAppend
)

type VFSType uint8

const (
	TypeUnknown VFSType = iota
	TypeFile
	TypeDir
)

type Info struct {
	Type VFSType
	Size uint32
}

type Writer struct {
	file tinyfs.File
}

type FS struct {
	lfs *tlfs.LFS
}

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

	dev := flashBlockDevice{flash: flash}
	lfs := tlfs.New(dev).Configure(&tlfs.Config{
		CacheSize:     opts.CacheSize,
		LookaheadSize: opts.LookaheadSize,
		BlockCycles:   opts.BlockCycles,
	})
	return &FS{lfs: lfs}, nil
}

func (fs *FS) Format() error {
	if fs == nil || fs.lfs == nil {
		return errors.New("littlefs: nil fs")
	}
	if err := fs.lfs.Format(); err != nil {
		return wrapErr("format", err)
	}
	return nil
}

func (fs *FS) Mount() error {
	if fs == nil || fs.lfs == nil {
		return errors.New("littlefs: nil fs")
	}
	if err := fs.lfs.Mount(); err != nil {
		return wrapErr("mount", err)
	}
	return nil
}

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

func (fs *FS) Mkdir(path string) error {
	if fs == nil || fs.lfs == nil {
		return errors.New("littlefs: nil fs")
	}
	if err := fs.lfs.Mkdir(path, 0o777); err != nil {
		return wrapErr("mkdir", err)
	}
	return nil
}

func (fs *FS) Remove(path string) error {
	if fs == nil || fs.lfs == nil {
		return errors.New("littlefs: nil fs")
	}
	if err := fs.lfs.Remove(path); err != nil {
		return wrapErr("remove", err)
	}
	return nil
}

func (fs *FS) Rename(oldPath, newPath string) error {
	if fs == nil || fs.lfs == nil {
		return errors.New("littlefs: nil fs")
	}
	if err := fs.lfs.Rename(oldPath, newPath); err != nil {
		return wrapErr("rename", err)
	}
	return nil
}

func (fs *FS) Stat(path string) (Info, error) {
	if fs == nil || fs.lfs == nil {
		return Info{}, errors.New("littlefs: nil fs")
	}
	fi, err := fs.lfs.Stat(path)
	if err != nil {
		return Info{}, wrapErr("stat", err)
	}
	typ := TypeFile
	if fi.IsDir() {
		typ = TypeDir
	}
	return Info{Type: typ, Size: uint32(fi.Size())}, nil
}

func (fs *FS) ListDir(path string, fn func(name string, info Info) bool) error {
	if fs == nil || fs.lfs == nil {
		return errors.New("littlefs: nil fs")
	}

	f, err := fs.lfs.OpenFile(path, os.O_RDONLY)
	if err != nil {
		return wrapErr("open dir", err)
	}
	defer func() { _ = f.Close() }()

	entries, err := f.Readdir(0)
	if err != nil {
		return wrapErr("readdir", err)
	}
	for _, e := range entries {
		name := e.Name()
		if name == "." || name == ".." {
			continue
		}
		typ := TypeFile
		if e.IsDir() {
			typ = TypeDir
		}
		if !fn(name, Info{Type: typ, Size: uint32(e.Size())}) {
			return nil
		}
	}
	return nil
}

func (fs *FS) ReadAt(path string, p []byte, off uint32) (n int, eof bool, err error) {
	if fs == nil || fs.lfs == nil {
		return 0, false, errors.New("littlefs: nil fs")
	}

	f, err := fs.lfs.OpenFile(path, os.O_RDONLY)
	if err != nil {
		return 0, false, wrapErr("open", err)
	}
	defer func() { _ = f.Close() }()

	if _, err := f.Seek(int64(off), io.SeekStart); err != nil {
		return 0, false, wrapErr("seek", err)
	}
	if len(p) == 0 {
		return 0, false, nil
	}
	n, err = f.Read(p)
	if err == nil {
		return n, n < len(p), nil
	}
	if errors.Is(err, io.EOF) {
		return n, true, nil
	}
	return n, false, wrapErr("read", err)
}

func (fs *FS) OpenWriter(path string, mode WriteMode) (*Writer, error) {
	if fs == nil || fs.lfs == nil {
		return nil, errors.New("littlefs: nil fs")
	}

	flags := os.O_WRONLY | os.O_CREATE
	switch mode {
	case WriteTruncate:
		flags |= os.O_TRUNC
	case WriteAppend:
		flags |= os.O_APPEND
	default:
		return nil, fmt.Errorf("littlefs open writer %q: invalid mode %d", path, mode)
	}

	f, err := fs.lfs.OpenFile(path, flags)
	if err != nil {
		return nil, wrapErr("open writer", err)
	}
	return &Writer{file: f}, nil
}

func (w *Writer) Write(p []byte) (int, error) {
	if w == nil || w.file == nil {
		return 0, errors.New("littlefs: write on closed writer")
	}
	return w.file.Write(p)
}

func (w *Writer) Close() error {
	if w == nil || w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	if err != nil {
		return wrapErr("close", err)
	}
	return nil
}

type flashBlockDevice struct {
	flash Flash
}

func (d flashBlockDevice) ReadAt(p []byte, off int64) (n int, err error) {
	if off < 0 || off > int64(^uint32(0)) {
		return 0, errors.New("littlefs: read offset out of range")
	}
	return d.flash.ReadAt(p, uint32(off))
}

func (d flashBlockDevice) WriteAt(p []byte, off int64) (n int, err error) {
	if off < 0 || off > int64(^uint32(0)) {
		return 0, errors.New("littlefs: write offset out of range")
	}
	return d.flash.WriteAt(p, uint32(off))
}

func (d flashBlockDevice) Size() int64 {
	return int64(d.flash.SizeBytes())
}

func (flashBlockDevice) WriteBlockSize() int64 {
	// RP2 flash can be programmed in 256-byte blocks.
	return 256
}

func (d flashBlockDevice) EraseBlockSize() int64 {
	return int64(d.flash.EraseBlockBytes())
}

func (d flashBlockDevice) EraseBlocks(start, length int64) error {
	if start < 0 || length < 0 || start > int64(^uint32(0)) || length > int64(^uint32(0)) {
		return errors.New("littlefs: erase range out of range")
	}
	bs := uint32(d.flash.EraseBlockBytes())
	if bs == 0 {
		return errors.New("littlefs: erase block size is zero")
	}
	return d.flash.Erase(uint32(start)*bs, uint32(length)*bs)
}

const (
	lfsErrNoEnt    = -2
	lfsErrExist    = -17
	lfsErrNotDir   = -20
	lfsErrIsDir    = -21
	lfsErrNotEmpty = -39
	lfsErrInvalid  = -22
	lfsErrNoSpc    = -28
	lfsErrCorrupt  = -84
)

func wrapErr(op string, err error) error {
	if err == nil {
		return nil
	}

	var lerr tlfs.Error
	if errors.As(err, &lerr) {
		switch int(lerr) {
		case lfsErrNoEnt:
			return fmt.Errorf("littlefs %s: %w", op, ErrNotFound)
		case lfsErrExist:
			return fmt.Errorf("littlefs %s: %w", op, ErrExists)
		case lfsErrNotDir:
			return fmt.Errorf("littlefs %s: %w", op, ErrNotDir)
		case lfsErrIsDir:
			return fmt.Errorf("littlefs %s: %w", op, ErrIsDir)
		case lfsErrNotEmpty:
			return fmt.Errorf("littlefs %s: %w", op, ErrNotEmpty)
		case lfsErrNoSpc:
			return fmt.Errorf("littlefs %s: %w", op, ErrNoSpace)
		case lfsErrInvalid:
			return fmt.Errorf("littlefs %s: %w", op, ErrInvalid)
		case lfsErrCorrupt:
			return fmt.Errorf("littlefs %s: %w", op, ErrCorrupt)
		default:
			return fmt.Errorf("littlefs %s: %v", op, err)
		}
	}

	return fmt.Errorf("littlefs %s: %v", op, err)
}
