package archive

import (
	"encoding/binary"
	"fmt"
	"strings"

	vfsclient "spark/sparkos/client/vfs"
	"spark/sparkos/kernel"
)

type zipCentralEntry struct {
	name string

	crc32 uint32
	size  uint32

	localOff          uint32
	isDir             bool
	useDataDescriptor bool
}

type zipStoreWriter struct {
	w *vfsclient.Writer

	off uint32

	entries []zipCentralEntry
}

func newZipStoreWriter(w *vfsclient.Writer) *zipStoreWriter {
	return &zipStoreWriter{w: w}
}

func (z *zipStoreWriter) write(p []byte) error {
	if len(p) == 0 {
		return nil
	}
	n, err := z.w.Write(p)
	z.off += uint32(n)
	return err
}

func (z *zipStoreWriter) AddDir(name string) error {
	name = sanitizeRelPath(name)
	if name == "" {
		return nil
	}
	if !strings.HasSuffix(name, "/") {
		name += "/"
	}
	localOff := z.off

	var hdr [30]byte
	binary.LittleEndian.PutUint32(hdr[0:4], zipLocalSig)
	binary.LittleEndian.PutUint16(hdr[4:6], 20)
	binary.LittleEndian.PutUint16(hdr[6:8], 0)
	binary.LittleEndian.PutUint16(hdr[8:10], 0) // store
	binary.LittleEndian.PutUint32(hdr[14:18], 0)
	binary.LittleEndian.PutUint32(hdr[18:22], 0)
	binary.LittleEndian.PutUint32(hdr[22:26], 0)
	binary.LittleEndian.PutUint16(hdr[26:28], uint16(len(name)))
	binary.LittleEndian.PutUint16(hdr[28:30], 0)

	if err := z.write(hdr[:]); err != nil {
		return err
	}
	if err := z.write([]byte(name)); err != nil {
		return err
	}
	z.entries = append(z.entries, zipCentralEntry{name: name, localOff: localOff, isDir: true})
	return nil
}

func (z *zipStoreWriter) AddFile(ctx *kernel.Context, vfs *vfsclient.Client, name, path string, size uint32) error {
	name = sanitizeRelPath(name)
	if name == "" {
		return fmt.Errorf("zip: empty name")
	}
	if strings.HasSuffix(name, "/") {
		return z.AddDir(name)
	}
	localOff := z.off

	// Use data descriptor (flag bit 3) so we can stream without precomputing CRC.
	const flagDataDescriptor = 0x08

	var hdr [30]byte
	binary.LittleEndian.PutUint32(hdr[0:4], zipLocalSig)
	binary.LittleEndian.PutUint16(hdr[4:6], 20)
	binary.LittleEndian.PutUint16(hdr[6:8], flagDataDescriptor)
	binary.LittleEndian.PutUint16(hdr[8:10], 0) // store
	binary.LittleEndian.PutUint32(hdr[14:18], 0)
	binary.LittleEndian.PutUint32(hdr[18:22], 0)
	binary.LittleEndian.PutUint32(hdr[22:26], 0)
	binary.LittleEndian.PutUint16(hdr[26:28], uint16(len(name)))
	binary.LittleEndian.PutUint16(hdr[28:30], 0)

	if err := z.write(hdr[:]); err != nil {
		return err
	}
	if err := z.write([]byte(name)); err != nil {
		return err
	}

	var crc uint32 = 0xffffffff
	var off uint32
	remain := size
	for remain > 0 {
		n := uint16(768)
		if remain < uint32(n) {
			n = uint16(remain)
		}
		b, _, err := vfs.ReadAt(ctx, path, off, n)
		if err != nil {
			return err
		}
		if len(b) == 0 {
			return fmt.Errorf("zip: unexpected EOF")
		}
		crc = crc32UpdateIEEE(crc, b)
		if err := z.write(b); err != nil {
			return err
		}
		off += uint32(len(b))
		remain -= uint32(len(b))
	}
	crc = ^crc

	// Data descriptor: signature + crc + sizes.
	var dd [16]byte
	binary.LittleEndian.PutUint32(dd[0:4], 0x08074b50)
	binary.LittleEndian.PutUint32(dd[4:8], crc)
	binary.LittleEndian.PutUint32(dd[8:12], size)
	binary.LittleEndian.PutUint32(dd[12:16], size)
	if err := z.write(dd[:]); err != nil {
		return err
	}

	z.entries = append(z.entries, zipCentralEntry{
		name:              name,
		crc32:             crc,
		size:              size,
		localOff:          localOff,
		isDir:             false,
		useDataDescriptor: true,
	})
	return nil
}

func (z *zipStoreWriter) Close() error {
	centralOff := z.off
	var cd []byte
	for _, e := range z.entries {
		var h [46]byte
		binary.LittleEndian.PutUint32(h[0:4], zipCentralSig)
		binary.LittleEndian.PutUint16(h[4:6], 20)
		binary.LittleEndian.PutUint16(h[6:8], 20)
		flags := uint16(0)
		if e.useDataDescriptor {
			flags |= 0x08
		}
		binary.LittleEndian.PutUint16(h[8:10], flags)
		binary.LittleEndian.PutUint16(h[10:12], 0) // store
		binary.LittleEndian.PutUint32(h[16:20], e.crc32)
		binary.LittleEndian.PutUint32(h[20:24], e.size)
		binary.LittleEndian.PutUint32(h[24:28], e.size)
		binary.LittleEndian.PutUint16(h[28:30], uint16(len(e.name)))
		binary.LittleEndian.PutUint16(h[30:32], 0)
		binary.LittleEndian.PutUint16(h[32:34], 0)
		binary.LittleEndian.PutUint16(h[34:36], 0)
		binary.LittleEndian.PutUint16(h[36:38], 0)
		extAttr := uint32(0)
		if e.isDir {
			extAttr = 0x10
		}
		binary.LittleEndian.PutUint32(h[38:42], extAttr)
		binary.LittleEndian.PutUint32(h[42:46], e.localOff)
		cd = append(cd, h[:]...)
		cd = append(cd, []byte(e.name)...)
	}
	if err := z.write(cd); err != nil {
		return err
	}
	cdSize := uint32(len(cd))

	var eocd [22]byte
	binary.LittleEndian.PutUint32(eocd[0:4], zipEndSig)
	binary.LittleEndian.PutUint16(eocd[8:10], uint16(len(z.entries)))
	binary.LittleEndian.PutUint16(eocd[10:12], uint16(len(z.entries)))
	binary.LittleEndian.PutUint32(eocd[12:16], cdSize)
	binary.LittleEndian.PutUint32(eocd[16:20], centralOff)
	binary.LittleEndian.PutUint16(eocd[20:22], 0)
	return z.write(eocd[:])
}

func crc32UpdateIEEE(crc uint32, b []byte) uint32 {
	const poly = 0xedb88320
	for i := 0; i < len(b); i++ {
		crc ^= uint32(b[i])
		for j := 0; j < 8; j++ {
			if crc&1 != 0 {
				crc = (crc >> 1) ^ poly
			} else {
				crc >>= 1
			}
		}
	}
	return crc
}
