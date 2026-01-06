package term

import (
	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

// Write sends a best-effort payload to the terminal service.
func Write(ctx *kernel.Context, termCap kernel.Capability, payload []byte) kernel.SendResult {
	if ctx == nil {
		return kernel.SendErrInvalidFromCap
	}
	if len(payload) > kernel.MaxMessageBytes {
		payload = payload[:kernel.MaxMessageBytes]
	}
	return ctx.SendToCapResult(termCap, uint16(proto.MsgTermWrite), payload, kernel.Capability{})
}

// WriteString sends a best-effort string to the terminal service.
func WriteString(ctx *kernel.Context, termCap kernel.Capability, s string) kernel.SendResult {
	return Write(ctx, termCap, []byte(s))
}

// Clear requests a terminal reset/clear.
func Clear(ctx *kernel.Context, termCap kernel.Capability) kernel.SendResult {
	if ctx == nil {
		return kernel.SendErrInvalidFromCap
	}
	return ctx.SendToCapResult(termCap, uint16(proto.MsgTermClear), nil, kernel.Capability{})
}
