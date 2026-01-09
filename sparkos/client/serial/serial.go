package serial

import (
	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

// Subscribe registers a receive endpoint for serial data.
func Subscribe(ctx *kernel.Context, serialCap, rxCap kernel.Capability) kernel.SendResult {
	if ctx == nil {
		return kernel.SendErrInvalidFromCap
	}
	return ctx.SendToCapResult(serialCap, uint16(proto.MsgSerialSubscribe), nil, rxCap)
}

// Write sends bytes to the serial interface.
func Write(ctx *kernel.Context, serialCap kernel.Capability, payload []byte) kernel.SendResult {
	if ctx == nil {
		return kernel.SendErrInvalidFromCap
	}
	if len(payload) > kernel.MaxMessageBytes {
		payload = payload[:kernel.MaxMessageBytes]
	}
	return ctx.SendToCapResult(serialCap, uint16(proto.MsgSerialWrite), payload, kernel.Capability{})
}
