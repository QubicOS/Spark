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

type retryState struct {
	sleeping bool
}

var retryStates [256]retryState

// LogRetry sends a log line to the logger service, retrying on SendErrQueueFull.
//
// It uses the time service for backoff. The call is cooperative: if it returns
// (done=false, err=nil), the caller should return from Task.Step immediately.
func LogRetry(
	ctx *kernel.Context,
	timeCap kernel.Capability,
	logCap kernel.Capability,
	line string,
) (done bool, err error) {
	if ctx == nil {
		return false, fmt.Errorf("logger retry: nil context")
	}

	st := &retryStates[ctx.TaskID()]

	if st.sleeping {
		done, err := timeclient.Sleep(ctx, timeCap, 1)
		if err != nil {
			st.sleeping = false
			return false, fmt.Errorf("logger retry backoff: %w", err)
		}
		if !done {
			return false, nil
		}
		st.sleeping = false
	}

	res := Log(ctx, logCap, line)
	switch res {
	case kernel.SendOK:
		return true, nil
	case kernel.SendErrQueueFull:
		st.sleeping = true
		return false, nil
	default:
		return false, fmt.Errorf("logger send: %s", res)
	}
}
