//go:build !(tinygo && baremetal && picocalc)

package vfs

import "spark/sparkos/kernel"

func (s *Service) initSD(_ *kernel.Context) fsHandle {
	return nil
}
