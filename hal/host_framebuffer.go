//go:build !tinygo

package hal

import "sync"

type hostFramebuffer struct {
	mu     sync.Mutex
	width  int
	height int
	stride int
	buf    []byte // front (presented)
	back   []byte // back (writable)
}

func newHostFramebuffer(width, height int) *hostFramebuffer {
	stride := width * 2
	size := stride * height
	return &hostFramebuffer{
		width:  width,
		height: height,
		stride: stride,
		buf:    make([]byte, size),
		back:   make([]byte, size),
	}
}

func (f *hostFramebuffer) Width() int          { return f.width }
func (f *hostFramebuffer) Height() int         { return f.height }
func (f *hostFramebuffer) Format() PixelFormat { return PixelFormatRGB565 }
func (f *hostFramebuffer) StrideBytes() int    { return f.stride }
func (f *hostFramebuffer) Buffer() []byte      { return f.back }

func (f *hostFramebuffer) Present() error {
	f.mu.Lock()
	f.buf, f.back = f.back, f.buf
	f.mu.Unlock()
	return nil
}

func (f *hostFramebuffer) ClearRGB(r, g, b uint8) {
	pixel := rgb565(r, g, b)
	lo := byte(pixel)
	hi := byte(pixel >> 8)
	for i := 0; i < len(f.back); i += 2 {
		f.back[i] = lo
		f.back[i+1] = hi
	}
}

func (f *hostFramebuffer) snapshotRGB565(dst []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	copy(dst, f.buf)
}
