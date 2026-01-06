//go:build tinygo

package imgview

import (
	"bufio"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"

	"spark/sparkos/kernel"
)

func (t *Task) renderRaster(ctx *kernel.Context, path string) error {
	r := &vfsStreamReader{
		ctx:    ctx,
		client: t.vfsClient(),
		path:   path,
		limit:  maxImageBytes,
	}
	br := bufio.NewReaderSize(r, 2*maxVFSRead)

	img, _, err := image.Decode(br)
	if err != nil {
		return fmt.Errorf("imgview: decode %s: %w", path, err)
	}
	return t.drawImageScaled(img)
}

type vfsStreamReader struct {
	ctx    *kernel.Context
	client interface {
		ReadAt(ctx *kernel.Context, path string, off uint32, maxBytes uint16) ([]byte, bool, error)
	}
	path  string
	off   uint32
	buf   []byte
	eof   bool
	read  int
	limit int
}

func (r *vfsStreamReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.limit > 0 && r.read >= r.limit {
		return 0, errors.New("imgview: file too large")
	}

	for len(r.buf) == 0 {
		if r.eof {
			return 0, io.EOF
		}
		chunk, eof, err := r.client.ReadAt(r.ctx, r.path, r.off, maxVFSRead)
		if err != nil {
			return 0, err
		}
		if len(chunk) == 0 {
			if eof {
				r.eof = true
				return 0, io.EOF
			}
			return 0, errors.New("imgview: read returned no data")
		}
		r.buf = chunk
		r.off += uint32(len(chunk))
		r.eof = eof
	}

	n := copy(p, r.buf)
	r.buf = r.buf[n:]
	r.read += n
	if r.limit > 0 && r.read > r.limit {
		return n, errors.New("imgview: file too large")
	}
	return n, nil
}
