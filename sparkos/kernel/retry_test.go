package kernel

import (
	"testing"
	"time"
)

func TestMessagePayloadClampsLen(t *testing.T) {
	var msg Message
	msg.Len = MaxMessageBytes + 10
	if got := len(msg.Payload()); got != MaxMessageBytes {
		t.Fatalf("expected payload length %d, got %d", MaxMessageBytes, got)
	}
}

func TestSendToCapRetryZeroLimitDoesNotBlock(t *testing.T) {
	k := New()
	ep := k.NewEndpoint(RightSend | RightRecv)
	if !ep.Valid() {
		t.Fatal("expected valid capability")
	}

	ctx := &Context{k: k, taskID: 1}
	to := ep.Restrict(RightSend)

	for i := 0; i < mailboxSlots; i++ {
		if res := ctx.SendToCapResult(to, 1, []byte("x"), Capability{}); res != SendOK {
			t.Fatalf("expected SendOK filling queue, got %s", res)
		}
	}

	res := ctx.SendToCapRetry(to, 1, []byte("y"), Capability{}, 0)
	if res != SendErrQueueFull {
		t.Fatalf("expected SendErrQueueFull, got %s", res)
	}
}

func TestSendToCapRetrySucceedsAfterDrain(t *testing.T) {
	k := New()
	ep := k.NewEndpoint(RightSend | RightRecv)
	if !ep.Valid() {
		t.Fatal("expected valid capability")
	}

	ctx := &Context{k: k, taskID: 1}
	to := ep.Restrict(RightSend)
	ch, ok := ctx.RecvChan(ep.Restrict(RightRecv))
	if !ok || ch == nil {
		t.Fatal("expected recv channel")
	}

	for i := 0; i < mailboxSlots; i++ {
		if res := ctx.SendToCapResult(to, 1, []byte("x"), Capability{}); res != SendOK {
			t.Fatalf("expected SendOK filling queue, got %s", res)
		}
	}

	resultCh := make(chan SendResult, 1)
	go func() {
		resultCh <- ctx.SendToCapRetry(to, 1, []byte("y"), Capability{}, 5)
	}()

	<-ch
	go func() {
		for i := uint64(1); i <= 10; i++ {
			k.TickTo(i)
			time.Sleep(1 * time.Millisecond)
		}
	}()

	select {
	case res := <-resultCh:
		if res != SendOK {
			t.Fatalf("expected SendOK after drain, got %s", res)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for send retry")
	}
}

func TestSendToCapRetryRespectsLimit(t *testing.T) {
	k := New()
	ep := k.NewEndpoint(RightSend | RightRecv)
	if !ep.Valid() {
		t.Fatal("expected valid capability")
	}

	ctx := &Context{k: k, taskID: 1}
	to := ep.Restrict(RightSend)

	for i := 0; i < mailboxSlots; i++ {
		if res := ctx.SendToCapResult(to, 1, []byte("x"), Capability{}); res != SendOK {
			t.Fatalf("expected SendOK filling queue, got %s", res)
		}
	}

	resultCh := make(chan SendResult, 1)
	go func() {
		resultCh <- ctx.SendToCapRetry(to, 1, []byte("y"), Capability{}, 1)
	}()

	go func() {
		for i := uint64(1); i <= 10; i++ {
			k.TickTo(i)
			time.Sleep(1 * time.Millisecond)
		}
	}()

	select {
	case res := <-resultCh:
		if res != SendErrQueueFull {
			t.Fatalf("expected SendErrQueueFull, got %s", res)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for send retry")
	}
}
