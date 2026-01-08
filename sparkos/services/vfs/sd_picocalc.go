//go:build tinygo && baremetal && picocalc

package vfs

import (
	"errors"
	"fmt"
	"io"
	"os"

	"machine"

	"spark/sparkos/fs/littlefs"
	"spark/sparkos/kernel"

	"tinygo.org/x/drivers/sdcard"
	"tinygo.org/x/tinyfs"
	"tinygo.org/x/tinyfs/fatfs"
)

func (s *Service) initSD(_ *kernel.Context) fsHandle {
	sd := sdcard.New(machine.SPI0, machine.GP18, machine.GP19, machine.GP16, machine.GP17)
	if err := sd.Configure(); err != nil {
		return nil
	}

	fat := fatfs.New(&sd).Configure(&fatfs.Config{SectorSize: fatfs.SectorSize})
	if err := fat.Mount(); err != nil {
		// Do not auto-format removable media.
		return nil
	}

	return &sdFatFS{
		sd:  &sd,
		fat: fat,
	}
}

type sdFatFS struct {
	sd  *sdcard.Device
	fat *fatfs.FATFS
}

func (fs *sdFatFS) ListDir(path string, fn func(name string, info littlefs.Info) bool) error {
	if fs == nil || fs.fat == nil {
		return errors.New("sd: not ready")
	}
	f, err := fs.fat.OpenFile(path, os.O_RDONLY)
	if err != nil {
		return mapFatErr("open dir", err)
	}
	defer func() { _ = f.Close() }()

	entries, err := f.Readdir(0)
	if err != nil {
		return mapFatErr("readdir", err)
	}
	for _, e := range entries {
		name := e.Name()
		if name == "." || name == ".." {
			continue
		}
		typ := littlefs.TypeFile
		if e.IsDir() {
			typ = littlefs.TypeDir
		}
		if !fn(name, littlefs.Info{Type: typ, Size: uint32(e.Size())}) {
			return nil
		}
	}
	return nil
}

func (fs *sdFatFS) Mkdir(path string) error {
	if fs == nil || fs.fat == nil {
		return errors.New("sd: not ready")
	}
	return mapFatErr("mkdir", fs.fat.Mkdir(path, 0o777))
}

func (fs *sdFatFS) Remove(path string) error {
	if fs == nil || fs.fat == nil {
		return errors.New("sd: not ready")
	}
	return mapFatErr("remove", fs.fat.Remove(path))
}

func (fs *sdFatFS) Rename(oldPath, newPath string) error {
	if fs == nil || fs.fat == nil {
		return errors.New("sd: not ready")
	}
	return mapFatErr("rename", fs.fat.Rename(oldPath, newPath))
}

func (fs *sdFatFS) Stat(path string) (littlefs.Info, error) {
	if fs == nil || fs.fat == nil {
		return littlefs.Info{}, errors.New("sd: not ready")
	}
	fi, err := fs.fat.Stat(path)
	if err != nil {
		return littlefs.Info{}, mapFatErr("stat", err)
	}
	typ := littlefs.TypeFile
	if fi.IsDir() {
		typ = littlefs.TypeDir
	}
	return littlefs.Info{Type: typ, Size: uint32(fi.Size())}, nil
}

func (fs *sdFatFS) ReadAt(path string, p []byte, off uint32) (n int, eof bool, err error) {
	if fs == nil || fs.fat == nil {
		return 0, false, errors.New("sd: not ready")
	}
	f, err := fs.fat.OpenFile(path, os.O_RDONLY)
	if err != nil {
		return 0, false, mapFatErr("open", err)
	}
	defer func() { _ = f.Close() }()

	if _, err := f.Seek(int64(off), io.SeekStart); err != nil {
		return 0, false, mapFatErr("seek", err)
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
	return n, false, mapFatErr("read", err)
}

func (fs *sdFatFS) OpenWriter(path string, mode littlefs.WriteMode) (writeHandle, error) {
	if fs == nil || fs.fat == nil {
		return nil, errors.New("sd: not ready")
	}

	flags := os.O_WRONLY | os.O_CREATE
	switch mode {
	case littlefs.WriteTruncate:
		flags |= os.O_TRUNC
	case littlefs.WriteAppend:
		flags |= os.O_APPEND
	default:
		return nil, fmt.Errorf("sd open writer %q: invalid mode %d", path, mode)
	}

	f, err := fs.fat.OpenFile(path, flags)
	if err != nil {
		return nil, mapFatErr("open writer", err)
	}
	return &sdWriter{f: f}, nil
}

type sdWriter struct {
	f       tinyfs.File
	written uint32
}

func (w *sdWriter) Write(p []byte) (int, error) {
	if w == nil || w.f == nil {
		return 0, errors.New("sd: write on closed writer")
	}
	n, err := w.f.Write(p)
	w.written += uint32(n)
	return n, err
}

func (w *sdWriter) Close() error {
	if w == nil || w.f == nil {
		return nil
	}
	err := w.f.Close()
	w.f = nil
	return err
}

func (w *sdWriter) BytesWritten() uint32 {
	if w == nil {
		return 0
	}
	return w.written
}

func mapFatErr(op string, err error) error {
	if err == nil {
		return nil
	}

	var fr fatfs.FileResult
	if errors.As(err, &fr) {
		switch fr {
		case fatfs.FileResultNoFile, fatfs.FileResultNoPath:
			return fmt.Errorf("sd %s: %w", op, littlefs.ErrNotFound)
		case fatfs.FileResultExist:
			return fmt.Errorf("sd %s: %w", op, littlefs.ErrExists)
		case fatfs.FileResultDenied, fatfs.FileResultLocked:
			return fmt.Errorf("sd %s: %w", op, littlefs.ErrNotEmpty)
		case fatfs.FileResultNoFilesystem, fatfs.FileResultInvalidName, fatfs.FileResultInvalidParameter:
			return fmt.Errorf("sd %s: %w", op, littlefs.ErrInvalid)
		case fatfs.FileResultNotEnoughCore:
			return fmt.Errorf("sd %s: %w", op, littlefs.ErrNoSpace)
		default:
			return fmt.Errorf("sd %s: %v", op, err)
		}
	}

	return fmt.Errorf("sd %s: %v", op, err)
}
