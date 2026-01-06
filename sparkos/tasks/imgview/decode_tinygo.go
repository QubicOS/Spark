//go:build tinygo

package imgview

import (
	"errors"

	"spark/sparkos/kernel"
)

func (t *Task) renderRaster(_ *kernel.Context, _ string) error {
	return errors.New("imgview: png/jpeg not supported in tinygo builds")
}
