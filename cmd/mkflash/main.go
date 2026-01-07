//go:build !tinygo

package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"spark/sparkos/fs/littlefs"
)

const (
	defaultFlashPath = "Flash.bin"
	defaultFlashSize = 16 * 1024 * 1024
	defaultEraseSize = 4096
)

type flashFile struct {
	f         *os.File
	size      uint32
	eraseSize uint32

	scratch []byte
}

func openFlashFile(path string, size uint32, eraseSize uint32) (*flashFile, error) {
	if eraseSize == 0 || eraseSize%256 != 0 {
		return nil, fmt.Errorf("flash: invalid erase size %d", eraseSize)
	}
	if size == 0 || size%eraseSize != 0 {
		return nil, fmt.Errorf("flash: size %d not multiple of erase size %d", size, eraseSize)
	}

	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open flash file %q: %w", path, err)
	}

	if err := f.Truncate(int64(size)); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("truncate flash file %q to %d: %w", path, size, err)
	}

	ff := &flashFile{
		f:         f,
		size:      size,
		eraseSize: eraseSize,
		scratch:   make([]byte, eraseSize),
	}
	for i := range ff.scratch {
		ff.scratch[i] = 0xFF
	}

	if err := ff.Erase(0, size); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("erase flash file %q: %w", path, err)
	}

	return ff, nil
}

func (f *flashFile) Close() error { return f.f.Close() }

func (f *flashFile) SizeBytes() uint32       { return f.size }
func (f *flashFile) EraseBlockBytes() uint32 { return f.eraseSize }

func (f *flashFile) ReadAt(p []byte, off uint32) (int, error) {
	if off >= f.size {
		return 0, fmt.Errorf("flash read at %d: %w", off, os.ErrInvalid)
	}
	maxN := int(f.size - off)
	if len(p) > maxN {
		p = p[:maxN]
	}
	return f.f.ReadAt(p, int64(off))
}

func (f *flashFile) WriteAt(p []byte, off uint32) (int, error) {
	if off >= f.size {
		return 0, fmt.Errorf("flash write at %d: %w", off, os.ErrInvalid)
	}
	maxN := int(f.size - off)
	if len(p) > maxN {
		p = p[:maxN]
	}

	prev := make([]byte, len(p))
	if _, err := f.f.ReadAt(prev, int64(off)); err != nil && !errors.Is(err, io.EOF) {
		return 0, fmt.Errorf("flash read before write at %d: %w", off, err)
	}
	for i := range p {
		if prev[i]&p[i] != p[i] {
			return 0, errors.New("flash write requires erase")
		}
	}
	return f.f.WriteAt(p, int64(off))
}

func (f *flashFile) Erase(off, size uint32) error {
	if size == 0 {
		return nil
	}
	if off%f.eraseSize != 0 || size%f.eraseSize != 0 {
		return fmt.Errorf("flash erase off=%d size=%d: %w", off, size, os.ErrInvalid)
	}
	if off >= f.size || off+size > f.size {
		return fmt.Errorf("flash erase off=%d size=%d: %w", off, size, os.ErrInvalid)
	}
	for size > 0 {
		if _, err := f.f.WriteAt(f.scratch, int64(off)); err != nil {
			return fmt.Errorf("flash erase block at %d: %w", off, err)
		}
		off += f.eraseSize
		size -= f.eraseSize
	}
	return nil
}

func main() {
	var srcDir string
	var outPath string
	var flashSize uint
	var eraseSize uint
	flag.StringVar(&srcDir, "src", "", "Source directory to import into LittleFS.")
	flag.StringVar(&outPath, "out", defaultFlashPath, "Output flash image path.")
	flag.UintVar(&flashSize, "size", defaultFlashSize, "Flash image size (bytes).")
	flag.UintVar(&eraseSize, "erase", defaultEraseSize, "Erase block size (bytes).")
	flag.Parse()

	if srcDir == "" {
		fmt.Fprintln(os.Stderr, "error: -src is required")
		os.Exit(2)
	}
	if outPath == "" {
		fmt.Fprintln(os.Stderr, "error: -out is required")
		os.Exit(2)
	}

	if err := run(srcDir, outPath, uint32(flashSize), uint32(eraseSize)); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(srcDir string, outPath string, flashSize uint32, eraseSize uint32) error {
	srcDir = filepath.Clean(srcDir)
	st, err := os.Stat(srcDir)
	if err != nil {
		return fmt.Errorf("stat src %q: %w", srcDir, err)
	}
	if !st.IsDir() {
		return fmt.Errorf("src %q is not a directory", srcDir)
	}

	ff, err := openFlashFile(outPath, flashSize, eraseSize)
	if err != nil {
		return err
	}
	defer func() { _ = ff.Close() }()

	fsLFS, err := littlefs.New(ff, littlefs.Options{})
	if err != nil {
		return err
	}
	defer func() { _ = fsLFS.Close() }()

	if err := fsLFS.Format(); err != nil {
		return err
	}
	if err := fsLFS.Mount(); err != nil {
		return err
	}

	var dirs []string
	var files []string
	walkErr := filepath.WalkDir(srcDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == srcDir {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		lfsPath := "/" + filepath.ToSlash(rel)
		if entry.IsDir() {
			dirs = append(dirs, lfsPath)
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		files = append(files, lfsPath)
		return nil
	})
	if walkErr != nil {
		return fmt.Errorf("walk src %q: %w", srcDir, walkErr)
	}

	sort.Strings(dirs)
	sort.Strings(files)

	for _, d := range dirs {
		if err := fsLFS.Mkdir(d); err != nil && !errors.Is(err, littlefs.ErrExists) {
			return fmt.Errorf("mkdir %q: %w", d, err)
		}
	}

	for _, fpath := range files {
		hostPath := filepath.Join(srcDir, filepath.FromSlash(strings.TrimPrefix(fpath, "/")))
		if err := copyFile(fsLFS, hostPath, fpath); err != nil {
			return err
		}
	}

	if err := fsLFS.Unmount(); err != nil {
		return err
	}
	return nil
}

func copyFile(fsLFS *littlefs.FS, hostPath string, lfsPath string) error {
	in, err := os.Open(hostPath)
	if err != nil {
		return fmt.Errorf("open %q: %w", hostPath, err)
	}
	defer func() { _ = in.Close() }()

	w, err := fsLFS.OpenWriter(lfsPath, littlefs.WriteTruncate)
	if err != nil {
		return fmt.Errorf("open writer %q: %w", lfsPath, err)
	}

	buf := make([]byte, 32*1024)
	for {
		n, err := in.Read(buf)
		if n > 0 {
			wrote, werr := w.Write(buf[:n])
			if werr != nil {
				_ = w.Close()
				return fmt.Errorf("write %q: %w", lfsPath, werr)
			}
			if wrote != n {
				_ = w.Close()
				return fmt.Errorf("write %q: short write", lfsPath)
			}
		}
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) {
			break
		}
		_ = w.Close()
		return fmt.Errorf("read %q: %w", hostPath, err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("close %q: %w", lfsPath, err)
	}
	return nil
}
