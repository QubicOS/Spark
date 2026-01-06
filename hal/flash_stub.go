package hal

type stubFlash struct{}

func (stubFlash) SizeBytes() uint32       { return 0 }
func (stubFlash) EraseBlockBytes() uint32 { return 0 }

func (stubFlash) ReadAt(p []byte, off uint32) (int, error) {
	_ = p
	_ = off
	return 0, ErrNotImplemented
}

func (stubFlash) WriteAt(p []byte, off uint32) (int, error) {
	_ = p
	_ = off
	return 0, ErrNotImplemented
}

func (stubFlash) Erase(off, size uint32) error {
	_ = off
	_ = size
	return ErrNotImplemented
}

