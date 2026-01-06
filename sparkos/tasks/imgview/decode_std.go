//go:build !tinygo

package imgview

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"

	"spark/hal"
	"spark/sparkos/kernel"
)

func (t *Task) renderRaster(ctx *kernel.Context, path string) error {
	data, err := t.readAll(ctx, path, maxImageBytes)
	if err != nil {
		return fmt.Errorf("imgview: read %s: %w", path, err)
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("imgview: decode %s: %w", path, err)
	}
	return t.drawImageScaled(img)
}

func (t *Task) drawImageScaled(img image.Image) error {
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

	b := img.Bounds()
	srcW := b.Dx()
	srcH := b.Dy()
	if srcW <= 0 || srcH <= 0 {
		return errors.New("imgview: invalid image geometry")
	}

	switch src := img.(type) {
	case *image.RGBA:
		for y := 0; y < dstH; y++ {
			sy := b.Min.Y + int((int64(y)*int64(srcH))/int64(dstH))
			row := y * stride
			for x := 0; x < dstW; x++ {
				sx := b.Min.X + int((int64(x)*int64(srcW))/int64(dstW))
				i := src.PixOffset(sx, sy)
				r := src.Pix[i+0]
				g := src.Pix[i+1]
				bl := src.Pix[i+2]
				pix := rgb565(r, g, bl)
				off := row + x*2
				if off < 0 || off+1 >= len(fbBuf) {
					continue
				}
				fbBuf[off] = byte(pix)
				fbBuf[off+1] = byte(pix >> 8)
			}
		}
		return t.fb.Present()

	case *image.NRGBA:
		for y := 0; y < dstH; y++ {
			sy := b.Min.Y + int((int64(y)*int64(srcH))/int64(dstH))
			row := y * stride
			for x := 0; x < dstW; x++ {
				sx := b.Min.X + int((int64(x)*int64(srcW))/int64(dstW))
				i := src.PixOffset(sx, sy)
				r := src.Pix[i+0]
				g := src.Pix[i+1]
				bl := src.Pix[i+2]
				pix := rgb565(r, g, bl)
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

	for y := 0; y < dstH; y++ {
		sy := b.Min.Y + int((int64(y)*int64(srcH))/int64(dstH))
		row := y * stride
		for x := 0; x < dstW; x++ {
			sx := b.Min.X + int((int64(x)*int64(srcW))/int64(dstW))
			c := color.RGBAModel.Convert(img.At(sx, sy)).(color.RGBA)
			pix := rgb565(c.R, c.G, c.B)
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
