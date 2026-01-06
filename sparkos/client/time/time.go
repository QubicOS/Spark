package time

import (
	"fmt"

	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

type sleepState struct {
	replyCap kernel.Capability

	inFlight  bool
	waitingID uint32
	nextID    uint32
}

var sleepStates [256]sleepState

// Sleep requests a wakeup after dt ticks via the time service.
//
// The call is cooperative: if it returns (done=false, err=nil), the caller should return
// from Task.Step immediately to let the kernel block the task.
func Sleep(ctx *kernel.Context, timeCap kernel.Capability, dt uint32) (done bool, err error) {
	if ctx == nil {
		return false, fmt.Errorf("time sleep: nil context")
	}

	st := &sleepStates[ctx.TaskID()]
	if !st.replyCap.Valid() {
		st.replyCap = ctx.NewEndpoint(kernel.RightSend | kernel.RightRecv)
		if !st.replyCap.Valid() {
			return false, fmt.Errorf("time sleep: allocate reply endpoint")
		}
	}

	replySend := st.replyCap.Restrict(kernel.RightSend)
	replyRecv := st.replyCap.Restrict(kernel.RightRecv)
	if !replySend.Valid() || !replyRecv.Valid() {
		return false, fmt.Errorf("time sleep: invalid reply capability")
	}

	if !st.inFlight {
		st.nextID++
		if st.nextID == 0 {
			st.nextID++
		}
		st.waitingID = st.nextID

		payload := proto.SleepPayload(st.waitingID, dt)
		res := ctx.SendToCapResult(timeCap, uint16(proto.MsgSleep), payload, replySend)
		switch res {
		case kernel.SendOK:
			st.inFlight = true
		case kernel.SendErrQueueFull:
			ctx.BlockOnTick()
			return false, nil
		default:
			return false, fmt.Errorf("time sleep send: %s", res)
		}
	}

	msg, ok := ctx.Recv(replyRecv)
	if !ok {
		return false, nil
	}

	switch proto.Kind(msg.Kind) {
	case proto.MsgWake:
		reqID, ok := proto.DecodeWakePayload(msg.Data[:msg.Len])
		if !ok {
			st.inFlight = false
			return false, fmt.Errorf("time wake: bad payload")
		}
		if reqID != st.waitingID {
			return false, nil
		}
		st.inFlight = false
		return true, nil

	case proto.MsgError:
		code, ref, detail, ok := proto.DecodeErrorPayload(msg.Data[:msg.Len])
		if !ok {
			st.inFlight = false
			return false, fmt.Errorf("time error: bad payload")
		}

		if reqID, _, ok := proto.DecodeErrorDetailWithRequestID(detail); ok && reqID != st.waitingID {
			return false, nil
		}

		st.inFlight = false
		return true, fmt.Errorf("time error: code=%s ref=%s", code, ref)

	default:
		return false, nil
	}
}
