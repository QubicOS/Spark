package consolemux

import (
	"fmt"
	"sync"

	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

type statusState struct {
	mu       sync.Mutex
	replyCap kernel.Capability
	nextID   uint32
}

var statusStates [256]statusState

type Status struct {
	ActiveApp proto.AppID
	FocusApp  bool
	HasApp    bool
}

func GetStatus(ctx *kernel.Context, muxCap kernel.Capability) (Status, error) {
	if ctx == nil {
		return Status{}, fmt.Errorf("consolemux status: nil context")
	}
	if !muxCap.Valid() {
		return Status{}, fmt.Errorf("consolemux status: no capability")
	}

	st := &statusStates[ctx.TaskID()]
	st.mu.Lock()
	defer st.mu.Unlock()

	if !st.replyCap.Valid() {
		st.replyCap = ctx.NewEndpoint(kernel.RightSend | kernel.RightRecv)
		if !st.replyCap.Valid() {
			return Status{}, fmt.Errorf("consolemux status: allocate reply endpoint")
		}
	}

	replySend := st.replyCap.Restrict(kernel.RightSend)
	replyRecv := st.replyCap.Restrict(kernel.RightRecv)
	if !replySend.Valid() || !replyRecv.Valid() {
		return Status{}, fmt.Errorf("consolemux status: invalid reply capability")
	}

	st.nextID++
	if st.nextID == 0 {
		st.nextID++
	}
	requestID := st.nextID

	payload := proto.MuxStatusPayload(requestID)
	res := ctx.SendToCapRetry(muxCap, uint16(proto.MsgMuxStatus), payload, replySend, 500)
	switch res {
	case kernel.SendOK:
		goto waitReply
	case kernel.SendErrQueueFull:
		return Status{}, fmt.Errorf("consolemux status send: queue full")
	default:
		return Status{}, fmt.Errorf("consolemux status send: %s", res)
	}

waitReply:
	for {
		msg, ok := ctx.Recv(replyRecv)
		if !ok {
			return Status{}, fmt.Errorf("consolemux status: recv")
		}

		switch proto.Kind(msg.Kind) {
		case proto.MsgMuxStatusResp:
			reqID, activeApp, focusApp, hasApp, ok := proto.DecodeMuxStatusRespPayload(msg.Payload())
			if !ok {
				return Status{}, fmt.Errorf("consolemux status resp: bad payload")
			}
			if reqID != requestID {
				continue
			}
			return Status{ActiveApp: activeApp, FocusApp: focusApp, HasApp: hasApp}, nil

		case proto.MsgError:
			code, ref, detail, ok := proto.DecodeErrorPayload(msg.Payload())
			if !ok {
				return Status{}, fmt.Errorf("consolemux status error: bad payload")
			}
			if reqID, _, ok := proto.DecodeErrorDetailWithRequestID(detail); ok && reqID != requestID {
				continue
			}
			return Status{}, fmt.Errorf("consolemux status error: code=%s ref=%s", code, ref)

		default:
			continue
		}
	}
}
