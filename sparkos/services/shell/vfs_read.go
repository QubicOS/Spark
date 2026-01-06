package shell

import (
	"errors"

	"spark/sparkos/kernel"
)

func (s *Service) readFileAll(ctx *kernel.Context, abs string, maxBytes int) ([]byte, error) {
	if maxBytes <= 0 {
		return nil, errors.New("file too large")
	}

	const maxRead = kernel.MaxMessageBytes - 11
	var off uint32
	var buf []byte

	for {
		b, eof, err := s.vfsClient().ReadAt(ctx, abs, off, maxRead)
		if err != nil {
			return nil, err
		}
		if len(b) > 0 {
			if len(buf)+len(b) > maxBytes {
				return nil, errors.New("file too large")
			}
			buf = append(buf, b...)
			off += uint32(len(b))
		}
		if eof {
			return buf, nil
		}
	}
}
