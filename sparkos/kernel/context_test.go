package kernel

import "testing"

func TestContextRecvClosed(t *testing.T) {
	k := New()
	cap := k.NewEndpoint(RightSend | RightRecv)
	if !cap.Valid() {
		t.Fatal("expected valid capability")
	}

	ctx := &Context{k: k, taskID: 1}
	ch, ok := ctx.RecvChan(cap.Restrict(RightRecv))
	if !ok || ch == nil {
		t.Fatal("expected recv channel")
	}

	close(k.endpoints[cap.ep].ch)

	if _, ok := ctx.Recv(cap.Restrict(RightRecv)); ok {
		t.Fatal("expected Recv to fail after channel close")
	}
	if _, ok := ctx.TryRecv(cap.Restrict(RightRecv)); ok {
		t.Fatal("expected TryRecv to fail after channel close")
	}
}

func TestContextSendClosed(t *testing.T) {
	k := New()
	cap := k.NewEndpoint(RightSend | RightRecv)
	if !cap.Valid() {
		t.Fatal("expected valid capability")
	}

	ctx := &Context{k: k, taskID: 1}
	close(k.endpoints[cap.ep].ch)

	res := ctx.SendToCapResult(cap.Restrict(RightSend), 1, []byte("x"), Capability{})
	if res != SendErrNoEndpoint {
		t.Fatalf("expected SendErrNoEndpoint, got %s", res)
	}
}
