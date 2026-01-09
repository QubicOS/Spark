package imgview

import (
	"errors"
	"fmt"
	"strings"

	"spark/hal"
	vfsclient "spark/sparkos/client/vfs"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

const maxVFSRead = kernel.MaxMessageBytes - 11
const maxImageBytes = 4 * 1024 * 1024

type Task struct {
	disp hal.Display
	ep   kernel.Capability

	vfsCap kernel.Capability
	vfs    *vfsclient.Client

	fb hal.Framebuffer

	active bool
	muxCap kernel.Capability

	path string
}

func New(disp hal.Display, ep kernel.Capability, vfsCap kernel.Capability) *Task {
	return &Task{disp: disp, ep: ep, vfsCap: vfsCap}
}

func (t *Task) Run(ctx *kernel.Context) {
	ch, ok := ctx.RecvChan(t.ep)
	if !ok {
		return
	}
	if t.disp == nil {
		return
	}

	t.fb = t.disp.Framebuffer()
	if t.fb == nil {
		return
	}

	for msg := range ch {
		switch proto.Kind(msg.Kind) {
		case proto.MsgAppShutdown:
			t.unloadSession()
			return

		case proto.MsgAppControl:
			if msg.Cap.Valid() {
				t.muxCap = msg.Cap
			}
			active, ok := proto.DecodeAppControlPayload(msg.Payload())
			if !ok {
				continue
			}
			t.setActive(ctx, active)

		case proto.MsgAppSelect:
			appID, arg, ok := proto.DecodeAppSelectPayload(msg.Payload())
			if !ok || appID != proto.AppImgView {
				continue
			}
			t.path = arg
			if t.active {
				t.render(ctx)
			}

		case proto.MsgTermInput:
			if !t.active {
				continue
			}
			if t.handleInput(ctx, msg.Payload()) {
				t.requestExit(ctx)
			}
		}
	}
}

func (t *Task) setActive(ctx *kernel.Context, active bool) {
	if active == t.active {
		if !active {
			t.unloadSession()
		}
		return
	}
	t.active = active
	if !t.active {
		t.unloadSession()
		return
	}
	t.render(ctx)
}

func (t *Task) handleInput(ctx *kernel.Context, b []byte) (exit bool) {
	_ = ctx
	for _, c := range b {
		switch c {
		case 0x1b, 'q':
			return true
		}
	}
	return false
}

func (t *Task) requestExit(ctx *kernel.Context) {
	if !t.muxCap.Valid() {
		t.active = false
		t.unloadSession()
		return
	}
	_ = ctx.SendToCapRetry(t.muxCap, uint16(proto.MsgAppControl), proto.AppControlPayload(false), kernel.Capability{}, 500)
	t.active = false
	t.unloadSession()
}

func (t *Task) unloadSession() {
	t.active = false
	t.path = ""
	t.vfs = nil
}

func (t *Task) vfsClient() *vfsclient.Client {
	if t.vfs == nil {
		t.vfs = vfsclient.New(t.vfsCap)
	}
	return t.vfs
}

func (t *Task) render(ctx *kernel.Context) {
	t.fb.ClearRGB(30, 30, 30)
	_ = t.fb.Present()

	if t.path == "" {
		t.showError()
		return
	}

	if err := t.renderFile(ctx, t.path); err != nil {
		t.showError()
	}
}

func (t *Task) showError() {
	t.fb.ClearRGB(80, 0, 0)
	_ = t.fb.Present()
}

func (t *Task) renderFile(ctx *kernel.Context, path string) error {
	head := make([]byte, 16)
	if err := t.readExact(ctx, path, 0, head); err != nil {
		return fmt.Errorf("imgview: read header: %w", err)
	}

	switch detectFormat(head, path) {
	case formatBMP:
		return t.renderBMP(ctx, path)
	case formatPNG, formatJPEG:
		return t.renderRaster(ctx, path)
	default:
		return errors.New("imgview: unsupported format")
	}
}

type fileFormat uint8

const (
	formatUnknown fileFormat = iota
	formatBMP
	formatPNG
	formatJPEG
)

func detectFormat(head []byte, path string) fileFormat {
	if len(head) >= 2 && head[0] == 'B' && head[1] == 'M' {
		return formatBMP
	}
	if len(head) >= 8 && head[0] == 0x89 && head[1] == 'P' && head[2] == 'N' && head[3] == 'G' && head[4] == 0x0D && head[5] == 0x0A && head[6] == 0x1A && head[7] == 0x0A {
		return formatPNG
	}
	if len(head) >= 2 && head[0] == 0xFF && head[1] == 0xD8 {
		return formatJPEG
	}

	ext := strings.ToLower(pathExt(path))
	switch ext {
	case ".bmp":
		return formatBMP
	case ".png":
		return formatPNG
	case ".jpg", ".jpeg":
		return formatJPEG
	default:
		return formatUnknown
	}
}

func pathExt(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return ""
		}
		if p[i] == '.' {
			return p[i:]
		}
	}
	return ""
}

// renderBMP loads an uncompressed BMP (24/32bpp) from VFS and draws it scaled to the framebuffer.
func (t *Task) renderBMP(ctx *kernel.Context, path string) error {
	hdr := make([]byte, 54)
	if err := t.readExact(ctx, path, 0, hdr); err != nil {
		return fmt.Errorf("imgview: read header: %w", err)
	}
	if hdr[0] != 'B' || hdr[1] != 'M' {
		return errors.New("imgview: not a BMP")
	}

	pixelOff := leU32(hdr[10:14])
	dibSize := leU32(hdr[14:18])
	if dibSize < 40 {
		return fmt.Errorf("imgview: unsupported DIB header size: %d", dibSize)
	}

	srcW := leI32(hdr[18:22])
	srcH0 := leI32(hdr[22:26])
	if srcW <= 0 || srcH0 == 0 {
		return fmt.Errorf("imgview: invalid dimensions: %dx%d", srcW, srcH0)
	}
	topDown := srcH0 < 0
	srcH := srcH0
	if srcH < 0 {
		srcH = -srcH
	}

	planes := leU16(hdr[26:28])
	if planes != 1 {
		return fmt.Errorf("imgview: unsupported planes: %d", planes)
	}

	bpp := leU16(hdr[28:30])
	if bpp != 24 && bpp != 32 {
		return fmt.Errorf("imgview: unsupported bpp: %d", bpp)
	}

	compression := leU32(hdr[30:34])
	if compression != 0 {
		return fmt.Errorf("imgview: unsupported compression: %d", compression)
	}

	rowBytes := ((uint32(bpp)*uint32(srcW) + 31) / 32) * 4
	if rowBytes == 0 {
		return errors.New("imgview: invalid row size")
	}
	rowBuf := make([]byte, rowBytes)

	if t.fb.Format() != hal.PixelFormatRGB565 {
		return errors.New("imgview: unsupported framebuffer format")
	}
	fbBuf := t.fb.Buffer()
	if fbBuf == nil {
		return errors.New("imgview: framebuffer buffer is nil")
	}

	dstW := t.fb.Width()
	dstH := t.fb.Height()
	stride := t.fb.StrideBytes()
	if dstW <= 0 || dstH <= 0 || stride <= 0 {
		return errors.New("imgview: invalid framebuffer geometry")
	}

	bytesPerPixel := int(bpp / 8)

	lastSrcRow := -1
	for y := 0; y < dstH; y++ {
		sy := int((int64(y) * int64(srcH)) / int64(dstH))
		if sy < 0 {
			sy = 0
		} else if sy >= srcH {
			sy = srcH - 1
		}

		srcRow := sy
		if !topDown {
			srcRow = srcH - 1 - sy
		}

		if srcRow != lastSrcRow {
			off := pixelOff + uint32(srcRow)*rowBytes
			if err := t.readExact(ctx, path, off, rowBuf); err != nil {
				return fmt.Errorf("imgview: read pixels: %w", err)
			}
			lastSrcRow = srcRow
		}

		row := y * stride
		for x := 0; x < dstW; x++ {
			sx := int((int64(x) * int64(srcW)) / int64(dstW))
			if sx < 0 {
				sx = 0
			} else if sx >= srcW {
				sx = srcW - 1
			}

			p := sx * bytesPerPixel
			if p+2 >= len(rowBuf) {
				continue
			}
			b := rowBuf[p+0]
			g := rowBuf[p+1]
			r := rowBuf[p+2]

			pix := rgb565(r, g, b)
			off := row + x*2
			if off < 0 || off+1 >= len(fbBuf) {
				continue
			}
			fbBuf[off] = byte(pix)
			fbBuf[off+1] = byte(pix >> 8)
		}
	}

	return t.fb.Present()
}

func (t *Task) readExact(ctx *kernel.Context, path string, off uint32, dst []byte) error {
	for len(dst) > 0 {
		n := len(dst)
		if n > maxVFSRead {
			n = maxVFSRead
		}

		chunk, eof, err := t.vfsClient().ReadAt(ctx, path, off, uint16(n))
		if err != nil {
			return err
		}
		if len(chunk) == 0 {
			if eof {
				return errors.New("unexpected EOF")
			}
			return errors.New("read returned no data")
		}

		copy(dst, chunk)
		dst = dst[len(chunk):]
		off += uint32(len(chunk))
		if eof && len(dst) > 0 {
			return errors.New("unexpected EOF")
		}
	}
	return nil
}

func (t *Task) readAll(ctx *kernel.Context, path string, maxBytes int) ([]byte, error) {
	if maxBytes <= 0 {
		maxBytes = maxImageBytes
	}

	var out []byte
	var off uint32
	for {
		chunk, eof, err := t.vfsClient().ReadAt(ctx, path, off, maxVFSRead)
		if err != nil {
			return nil, err
		}
		if len(chunk) > 0 {
			out = append(out, chunk...)
			if len(out) > maxBytes {
				return nil, errors.New("imgview: file too large")
			}
			off += uint32(len(chunk))
		}
		if eof {
			return out, nil
		}
		if len(chunk) == 0 {
			return nil, errors.New("imgview: read returned no data")
		}
	}
}

func leU16(b []byte) uint16 {
	if len(b) < 2 {
		return 0
	}
	return uint16(b[0]) | uint16(b[1])<<8
}

func leU32(b []byte) uint32 {
	if len(b) < 4 {
		return 0
	}
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}

func leI32(b []byte) int {
	return int(int32(leU32(b)))
}

func rgb565(r, g, b uint8) uint16 {
	return uint16((uint16(r>>3)&0x1F)<<11 | (uint16(g>>2)&0x3F)<<5 | (uint16(b>>3) & 0x1F))
}
