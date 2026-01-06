package time

import (
	"fmt"

	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

type sleepState struct {
	replyCap kernel.Capability
	nextID   uint32
}

var sleepStates [256]sleepState

// Sleep requests a wakeup after dt ticks via the time service.
func Sleep(ctx *kernel.Context, timeCap kernel.Capability, dt uint32) error {
	if ctx == nil {
		return fmt.Errorf("time sleep: nil context")
	}

	st := &sleepStates[ctx.TaskID()]
	if !st.replyCap.Valid() {
		st.replyCap = ctx.NewEndpoint(kernel.RightSend | kernel.RightRecv)
		if !st.replyCap.Valid() {
			return fmt.Errorf("time sleep: allocate reply endpoint")
		}
	}

	replySend := st.replyCap.Restrict(kernel.RightSend)
	replyRecv := st.replyCap.Restrict(kernel.RightRecv)
	if !replySend.Valid() || !replyRecv.Valid() {
		return fmt.Errorf("time sleep: invalid reply capability")
	}

	st.nextID++
	if st.nextID == 0 {
		st.nextID++
	}
	requestID := st.nextID

	payload := proto.SleepPayload(requestID, dt)
	for {
		res := ctx.SendToCapResult(timeCap, uint16(proto.MsgSleep), payload, replySend)
		switch res {
		case kernel.SendOK:
			goto waitReply
		case kernel.SendErrQueueFull:
			ctx.BlockOnTick()
			continue
		default:
			return fmt.Errorf("time sleep send: %s", res)
		}
	}

waitReply:
	for {
		msg, ok := ctx.Recv(replyRecv)
		if !ok {
			return fmt.Errorf("time sleep: recv")
		}

		switch proto.Kind(msg.Kind) {
		case proto.MsgWake:
			reqID, ok := proto.DecodeWakePayload(msg.Data[:msg.Len])
			if !ok {
				return fmt.Errorf("time wake: bad payload")
			}
			if reqID != requestID {
				continue
			}
			return nil

		case proto.MsgError:
			code, ref, detail, ok := proto.DecodeErrorPayload(msg.Data[:msg.Len])
			if !ok {
				return fmt.Errorf("time error: bad payload")
			}

			if reqID, _, ok := proto.DecodeErrorDetailWithRequestID(detail); ok && reqID != requestID {
				continue
			}

			return fmt.Errorf("time error: code=%s ref=%s", code, ref)

		default:
			continue
		}
	}
}
