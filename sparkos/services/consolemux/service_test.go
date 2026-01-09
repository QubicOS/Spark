package consolemux

import (
	"testing"
	"time"

	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

const testTimeout = 1 * time.Second

type sendReq struct {
	kind    proto.Kind
	payload []byte
	done    chan<- kernel.SendResult
}

type senderTask struct {
	to   kernel.Capability
	reqs <-chan sendReq
}

func (t *senderTask) Run(ctx *kernel.Context) {
	for req := range t.reqs {
		res := ctx.SendToCapResult(t.to, uint16(req.kind), req.payload, kernel.Capability{})
		req.done <- res
	}
}

type recvTask struct {
	cap kernel.Capability
	out chan<- kernel.Message
}

func (t *recvTask) Run(ctx *kernel.Context) {
	ch, ok := ctx.RecvChan(t.cap)
	if !ok {
		return
	}
	for msg := range ch {
		t.out <- msg
	}
}

type serviceTask struct {
	svc *Service
}

func (t *serviceTask) Run(ctx *kernel.Context) {
	t.svc.Run(ctx)
}

func recvWithTimeout[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for message")
		var zero T
		return zero
	}
}

func sendTo(t *testing.T, ch chan<- sendReq, kind proto.Kind, payload []byte) {
	t.Helper()
	done := make(chan kernel.SendResult, 1)
	ch <- sendReq{kind: kind, payload: payload, done: done}
	res := recvWithTimeout(t, done)
	if res != kernel.SendOK {
		t.Fatalf("send %s: %s", kind, res)
	}
}

func TestCtrlGTogglesFocus(t *testing.T) {
	k := kernel.New()

	muxEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	shellEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	appEP := k.NewEndpoint(kernel.RightSend | kernel.RightRecv)

	muxIn := muxEP.Restrict(kernel.RightRecv)
	muxCtl := muxEP.Restrict(kernel.RightSend)

	shellSend := shellEP.Restrict(kernel.RightSend)
	shellRecv := shellEP.Restrict(kernel.RightRecv)

	appSend := appEP.Restrict(kernel.RightSend)
	appRecv := appEP.Restrict(kernel.RightRecv)

	if !muxIn.Valid() || !muxCtl.Valid() || !shellSend.Valid() || !shellRecv.Valid() || !appSend.Valid() || !appRecv.Valid() {
		t.Fatal("expected valid capabilities")
	}

	svc := New(
		muxIn,
		muxCtl,
		shellSend,
		appSend,
		kernel.Capability{},
		kernel.Capability{},
		kernel.Capability{},
		kernel.Capability{},
		kernel.Capability{},
		kernel.Capability{},
		kernel.Capability{},
		kernel.Capability{},
		kernel.Capability{},
		kernel.Capability{},
		kernel.Capability{},
		kernel.Capability{},
		kernel.Capability{},
		kernel.Capability{},
		kernel.Capability{},
	)
	k.AddTask(&serviceTask{svc: svc})

	shellOut := make(chan kernel.Message, 16)
	appOut := make(chan kernel.Message, 16)
	k.AddTask(&recvTask{cap: shellRecv, out: shellOut})
	k.AddTask(&recvTask{cap: appRecv, out: appOut})

	sendReqCh := make(chan sendReq, 16)
	k.AddTask(&senderTask{to: muxEP.Restrict(kernel.RightSend), reqs: sendReqCh})

	// Shell is active by default.
	sendTo(t, sendReqCh, proto.MsgTermInput, []byte("hello"))
	msg := recvWithTimeout(t, shellOut)
	if proto.Kind(msg.Kind) != proto.MsgTermInput {
		t.Fatalf("expected MsgTermInput, got %s", proto.Kind(msg.Kind))
	}
	if got := string(msg.Payload()); got != "hello" {
		t.Fatalf("expected shell payload %q, got %q", "hello", got)
	}

	// Ctrl+G activates the app and transfers the control cap.
	sendTo(t, sendReqCh, proto.MsgTermInput, []byte{interruptByte})
	msg = recvWithTimeout(t, appOut)
	if proto.Kind(msg.Kind) != proto.MsgAppControl {
		t.Fatalf("expected MsgAppControl to app, got %s", proto.Kind(msg.Kind))
	}
	active, ok := proto.DecodeAppControlPayload(msg.Payload())
	if !ok || !active {
		t.Fatalf("expected app active=true, got active=%v ok=%v", active, ok)
	}
	if msg.Cap != muxCtl {
		t.Fatal("expected transferred control capability")
	}
	msg = recvWithTimeout(t, shellOut)
	if proto.Kind(msg.Kind) != proto.MsgAppControl {
		t.Fatalf("expected MsgAppControl to shell, got %s", proto.Kind(msg.Kind))
	}
	active, ok = proto.DecodeAppControlPayload(msg.Payload())
	if !ok || active {
		t.Fatalf("expected shell active=false, got active=%v ok=%v", active, ok)
	}
	if msg.Cap.Valid() {
		t.Fatal("did not expect capability transfer to shell")
	}

	// Now input goes to the app.
	sendTo(t, sendReqCh, proto.MsgTermInput, []byte("x"))
	msg = recvWithTimeout(t, appOut)
	if proto.Kind(msg.Kind) != proto.MsgTermInput {
		t.Fatalf("expected MsgTermInput to app, got %s", proto.Kind(msg.Kind))
	}
	if got := string(msg.Payload()); got != "x" {
		t.Fatalf("expected app payload %q, got %q", "x", got)
	}

	// Ctrl+G toggles back to shell; remaining bytes go to shell.
	sendTo(t, sendReqCh, proto.MsgTermInput, []byte{interruptByte, 'y'})

	msg = recvWithTimeout(t, appOut)
	if proto.Kind(msg.Kind) != proto.MsgAppControl {
		t.Fatalf("expected MsgAppControl to app, got %s", proto.Kind(msg.Kind))
	}
	active, ok = proto.DecodeAppControlPayload(msg.Payload())
	if !ok || active {
		t.Fatalf("expected app active=false, got active=%v ok=%v", active, ok)
	}
	if msg.Cap.Valid() {
		t.Fatal("did not expect capability transfer while deactivating app")
	}

	msg = recvWithTimeout(t, shellOut)
	if proto.Kind(msg.Kind) != proto.MsgAppControl {
		t.Fatalf("expected MsgAppControl to shell, got %s", proto.Kind(msg.Kind))
	}
	active, ok = proto.DecodeAppControlPayload(msg.Payload())
	if !ok || !active {
		t.Fatalf("expected shell active=true, got active=%v ok=%v", active, ok)
	}
	if msg.Cap.Valid() {
		t.Fatal("did not expect capability transfer to shell")
	}

	msg = recvWithTimeout(t, shellOut)
	if proto.Kind(msg.Kind) != proto.MsgTermInput {
		t.Fatalf("expected MsgTermInput to shell, got %s", proto.Kind(msg.Kind))
	}
	if got := string(msg.Payload()); got != "y" {
		t.Fatalf("expected shell payload %q, got %q", "y", got)
	}
}
