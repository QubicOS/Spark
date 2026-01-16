//go:build tinygo && !baremetal

package hal

type tinyGoHostFramebuffer struct {
	w      int
	h      int
	stride int
	buf    []byte
}

func newTinyGoHostFramebuffer(w, h int) *tinyGoHostFramebuffer {
	stride := w * 2
	return &tinyGoHostFramebuffer{
		w:      w,
		h:      h,
		stride: stride,
		buf:    make([]byte, stride*h),
	}
}

func (f *tinyGoHostFramebuffer) Width() int          { return f.w }
func (f *tinyGoHostFramebuffer) Height() int         { return f.h }
func (f *tinyGoHostFramebuffer) Format() PixelFormat { return PixelFormatRGB565 }
func (f *tinyGoHostFramebuffer) StrideBytes() int    { return f.stride }
func (f *tinyGoHostFramebuffer) Buffer() []byte      { return f.buf }

func (f *tinyGoHostFramebuffer) ClearRGB(r, g, b uint8) {
	pixel := rgb565(r, g, b)
	lo := byte(pixel)
	hi := byte(pixel >> 8)
	for i := 0; i < len(f.buf); i += 2 {
		f.buf[i] = lo
		f.buf[i+1] = hi
	}
}

func (f *tinyGoHostFramebuffer) Present() error {
	// No-op by default for tinygo host targets.
	return nil
}
