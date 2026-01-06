package logger

import (
	"fmt"

	timeclient "spark/sparkos/client/time"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

// Log sends a log line to the logger service.
//
// The call is best-effort: it may drop on queue full.
func Log(ctx *kernel.Context, logCap kernel.Capability, line string) kernel.SendResult {
	if ctx == nil {
		return kernel.SendErrInvalidFromCap
	}
	b := []byte(line)
	if len(b) > kernel.MaxMessageBytes {
		b = b[:kernel.MaxMessageBytes]
	}
	return ctx.SendToCapResult(logCap, uint16(proto.MsgLogLine), proto.LogLinePayload(b), kernel.Capability{})
}

// LogRetry sends a log line to the logger service, retrying on SendErrQueueFull.
//
// It uses the time service for backoff.
func LogRetry(
	ctx *kernel.Context,
	timeCap kernel.Capability,
	logCap kernel.Capability,
	line string,
) error {
	if ctx == nil {
		return fmt.Errorf("logger retry: nil context")
	}

	for {
		res := Log(ctx, logCap, line)
		switch res {
		case kernel.SendOK:
			return nil
		case kernel.SendErrQueueFull:
			if err := timeclient.Sleep(ctx, timeCap, 1); err != nil {
				return fmt.Errorf("logger retry backoff: %w", err)
			}
		default:
			return fmt.Errorf("logger send: %s", res)
		}
	}
}
