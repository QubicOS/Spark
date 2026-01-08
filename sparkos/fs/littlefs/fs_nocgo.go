//go:build !tinygo && !cgo

package littlefs

import "errors"

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
	// ErrNotEmpty indicates that a directory is not empty.
	ErrNotEmpty = errors.New("littlefs: directory not empty")
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

type Type uint8

const (
	TypeFile Type = iota + 1
	TypeDir
)

type Info struct {
	Type Type
	Size uint32
}

type FS struct{}

func New(_ Flash, _ Options) (*FS, error) {
	return nil, errors.New("littlefs: requires cgo")
}

func (fs *FS) Close() error         { return nil }
func (fs *FS) Mount() error         { return errors.New("littlefs: requires cgo") }
func (fs *FS) Unmount() error       { return nil }
func (fs *FS) Format() error        { return errors.New("littlefs: requires cgo") }
func (fs *FS) MountOrFormat() error { return errors.New("littlefs: requires cgo") }
func (fs *FS) ListDir(string, func(string, Info) bool) error {
	return errors.New("littlefs: requires cgo")
}
func (fs *FS) Mkdir(string) error          { return errors.New("littlefs: requires cgo") }
func (fs *FS) Remove(string) error         { return errors.New("littlefs: requires cgo") }
func (fs *FS) Rename(string, string) error { return errors.New("littlefs: requires cgo") }
func (fs *FS) Stat(string) (Info, error)   { return Info{}, errors.New("littlefs: requires cgo") }
func (fs *FS) ReadAt(string, []byte, uint32) (int, bool, error) {
	return 0, false, errors.New("littlefs: requires cgo")
}
func (fs *FS) OpenWriter(string, WriteMode) (*Writer, error) {
	return nil, errors.New("littlefs: requires cgo")
}

type Writer struct{}

func (w *Writer) Write([]byte) (int, error) { return 0, errors.New("littlefs: requires cgo") }
func (w *Writer) Close() error              { return nil }
func (w *Writer) BytesWritten() uint32      { return 0 }
