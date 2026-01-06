package proto

// Kind identifies the message type carried in kernel.Message.Kind.
type Kind uint16

const (
	MsgLogLine Kind = iota + 1
	MsgSleep
	MsgWake
	MsgError
	MsgTermWrite
	MsgTermClear
	MsgTermInput
)

// ErrCode is a generic error category for MsgError responses.
type ErrCode uint16

const (
	ErrUnknown ErrCode = iota
	ErrBadMessage
	ErrUnauthorized
	ErrNotFound
	ErrBusy
	ErrOverflow
	ErrTooLarge
	ErrInternal
)

func (c ErrCode) String() string {
	switch c {
	case ErrUnknown:
		return "unknown"
	case ErrBadMessage:
		return "bad_message"
	case ErrUnauthorized:
		return "unauthorized"
	case ErrNotFound:
		return "not_found"
	case ErrBusy:
		return "busy"
	case ErrOverflow:
		return "overflow"
	case ErrTooLarge:
		return "too_large"
	case ErrInternal:
		return "internal"
	default:
		return "unknown"
	}
}

func (k Kind) String() string {
	switch k {
	case MsgLogLine:
		return "log_line"
	case MsgSleep:
		return "sleep"
	case MsgWake:
		return "wake"
	case MsgError:
		return "error"
	case MsgTermWrite:
		return "term_write"
	case MsgTermClear:
		return "term_clear"
	case MsgTermInput:
		return "term_input"
	default:
		return "unknown"
	}
}
