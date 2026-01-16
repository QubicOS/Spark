package hal

type nullNetwork struct{}

func (nullNetwork) Send(pkt []byte) error {
	_ = pkt
	return ErrNotImplemented
}

func (nullNetwork) Recv(pkt []byte) (int, error) {
	_ = pkt
	return 0, ErrNotImplemented
}
