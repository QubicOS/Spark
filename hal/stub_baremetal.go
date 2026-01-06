//go:build tinygo && baremetal

package hal

type stubFramebuffer struct {
	w      int
	h      int
	format PixelFormat
}

func (f *stubFramebuffer) Width() int          { return f.w }
func (f *stubFramebuffer) Height() int         { return f.h }
func (f *stubFramebuffer) Format() PixelFormat { return f.format }
func (f *stubFramebuffer) StrideBytes() int    { return f.w * 2 }
func (f *stubFramebuffer) Buffer() []byte      { return nil }
func (f *stubFramebuffer) ClearRGB(r, g, b uint8) {
	_ = r
	_ = g
	_ = b
}
func (f *stubFramebuffer) Present() error { return ErrNotImplemented }

type stubKeyboard struct{}

func (k *stubKeyboard) Events() <-chan KeyEvent { return nil }

