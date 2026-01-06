//go:build tinygo

package littlefs

import "errors"

var (
	ErrNotMounted = errors.New("littlefs: not mounted")
	ErrNotFound   = errors.New("littlefs: not found")
	ErrExists     = errors.New("littlefs: already exists")
	ErrNotDir     = errors.New("littlefs: not a directory")
	ErrIsDir      = errors.New("littlefs: is a directory")
	ErrNoSpace    = errors.New("littlefs: no space")
	ErrInvalid    = errors.New("littlefs: invalid")
	ErrCorrupt    = errors.New("littlefs: corrupt")
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

type Writer struct{}

type FS struct{}

func New(flash Flash, opts Options) (*FS, error) {
	_ = flash
	_ = opts
	return nil, errors.New("littlefs: not implemented on tinygo yet")
}
