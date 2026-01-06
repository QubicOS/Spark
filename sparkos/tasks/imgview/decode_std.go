//go:build !tinygo

package imgview

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"

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
