package kernel

// Endpoint identifies a message destination.
type Endpoint uint8

const (
	EPKernel Endpoint = iota
	EPLogger
	EPPing
	EPPong
)

