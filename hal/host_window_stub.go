//go:build !tinygo && !cgo

package hal

import "errors"

func RunWindow(_ func(h HAL) func() error) error {
	return errors.New("window mode requires cgo (build/run with CGO_ENABLED=1)")
}
