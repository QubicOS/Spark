package archive

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

type archiveKind uint8

const (
	archiveNone archiveKind = iota
	archiveTar
	archiveZip
	archiveTarGz
)

func (k archiveKind) String() string {
	switch k {
	case archiveTar:
		return "tar"
	case archiveZip:
		return "zip"
	case archiveTarGz:
		return "tar.gz"
	default:
		return "unknown"
	}
}

func detectArchiveKind(path string, head []byte) archiveKind {
	lower := strings.ToLower(strings.TrimSpace(path))
	switch {
	case strings.HasSuffix(lower, ".tar"):
		return archiveTar
	case strings.HasSuffix(lower, ".zip"):
		return archiveZip
	case strings.HasSuffix(lower, ".tgz") || strings.HasSuffix(lower, ".tar.gz"):
		return archiveTarGz
	}

	// zip local header signature.
	if len(head) >= 4 && bytes.Equal(head[:4], []byte{0x50, 0x4b, 0x03, 0x04}) {
		return archiveZip
	}
	// gzip magic.
	if len(head) >= 2 && head[0] == 0x1f && head[1] == 0x8b {
		return archiveTarGz
	}
	return archiveTar
}

type entryType uint8

const (
	entryFile entryType = iota
	entryDir
)

type entry struct {
	name string
	typ  entryType

	size uint32

	// Tar: dataOff points to file content, size is file size.
	// Zip: dataOff points to file content (after local header).
	dataOff uint32

	// Zip:
	compSize   uint32
	compMethod uint16
}

var (
	errUnsupportedZipMethod = errors.New("zip: unsupported compression (only store is supported)")
)

func u32le(b []byte) uint32 { return binary.LittleEndian.Uint32(b) }
func u16le(b []byte) uint16 { return binary.LittleEndian.Uint16(b) }

func sanitizeRelPath(s string) string {
	s = strings.ReplaceAll(s, "\\", "/")
	s = strings.TrimSpace(s)
	for strings.HasPrefix(s, "/") {
		s = strings.TrimPrefix(s, "/")
	}
	for strings.HasPrefix(s, "./") {
		s = strings.TrimPrefix(s, "./")
	}
	parts := strings.Split(s, "/")
	out := parts[:0]
	for _, p := range parts {
		if p == "" || p == "." {
			continue
		}
		if p == ".." {
			if len(out) > 0 {
				out = out[:len(out)-1]
			}
			continue
		}
		out = append(out, p)
	}
	return strings.Join(out, "/")
}

func joinPath(dir, rel string) string {
	dir = strings.TrimSpace(dir)
	if dir == "" || dir == "/" {
		dir = "/"
	}
	if !strings.HasPrefix(dir, "/") {
		dir = "/" + dir
	}
	dir = strings.TrimRight(dir, "/")
	rel = sanitizeRelPath(rel)
	if rel == "" {
		return dir
	}
	return dir + "/" + rel
}

func fmtBytes(n uint32) string {
	if n < 1024 {
		return fmt.Sprintf("%dB", n)
	}
	kb := float64(n) / 1024.0
	if kb < 1024 {
		return fmt.Sprintf("%.1fKB", kb)
	}
	mb := kb / 1024.0
	if mb < 1024 {
		return fmt.Sprintf("%.1fMB", mb)
	}
	gb := mb / 1024.0
	return fmt.Sprintf("%.1fGB", gb)
}
