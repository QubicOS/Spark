//go:build !tinygo

package hal

import "sync"

type hostFramebuffer struct {
	mu     sync.Mutex
	width  int
	height int
	stride int
	buf    []byte
}

func newHostFramebuffer(width, height int) *hostFramebuffer {
	stride := width * 2
	return &hostFramebuffer{
		width:  width,
		height: height,
		stride: stride,
		buf:    make([]byte, stride*height),
	}
}

func (f *hostFramebuffer) Width() int              { return f.width }
func (f *hostFramebuffer) Height() int             { return f.height }
func (f *hostFramebuffer) Format() PixelFormat     { return PixelFormatRGB565 }
func (f *hostFramebuffer) StrideBytes() int        { return f.stride }
func (f *hostFramebuffer) Buffer() []byte          { return f.buf }
func (f *hostFramebuffer) Present() error          { return nil }

func (f *hostFramebuffer) ClearRGB(r, g, b uint8) {
	f.mu.Lock()
	defer f.mu.Unlock()

	pixel := rgb565(r, g, b)
	lo := byte(pixel)
	hi := byte(pixel >> 8)
	for i := 0; i < len(f.buf); i += 2 {
		f.buf[i] = lo
		f.buf[i+1] = hi
	}
}

func (f *hostFramebuffer) snapshotRGB565(dst []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	copy(dst, f.buf)
}

