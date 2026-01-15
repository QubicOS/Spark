package kernel

import (
	"encoding/binary"
	"runtime"
	"sync"
	"testing"
)

func TestMailboxTryRecvEmpty(t *testing.T) {
	var mb Mailbox

	_, ok := mb.TryRecv()
	if ok {
		t.Fatalf("TryRecv() ok = true, want false")
	}
}

func TestMailboxTrySendFull(t *testing.T) {
	var mb Mailbox
	var msg Message

	for i := 0; i < mailboxSlots; i++ {
		if ok := mb.TrySend(msg); !ok {
			t.Fatalf("TrySend() ok = false at slot %d, want true", i)
		}
	}
	if ok := mb.TrySend(msg); ok {
		t.Fatalf("TrySend() ok = true when full, want false")
	}

	for i := 0; i < mailboxSlots; i++ {
		if _, ok := mb.TryRecv(); !ok {
			t.Fatalf("TryRecv() ok = false at slot %d, want true", i)
		}
	}
}

func TestMailboxConcurrentProducers(t *testing.T) {
	oldProcs := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(oldProcs)

	const (
		producers = 4
		perProd   = 10_000
		total     = producers * perProd
	)

	var mb Mailbox

	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(producers)
	for producerID := 0; producerID < producers; producerID++ {
		go func(producerID int) {
			defer wg.Done()
			<-start
			for i := 0; i < perProd; i++ {
				id := uint32(producerID*perProd + i)
				var msg Message
				msg.Len = 4
				binary.LittleEndian.PutUint32(msg.Data[:4], id)
				mb.Send(msg)
			}
		}(producerID)
	}
	close(start)

	seen := make([]bool, total)
	for i := 0; i < total; i++ {
		msg := mb.Recv()
		if msg.Len != 4 {
			t.Fatalf("Recv() msg.Len = %d, want 4", msg.Len)
		}
		id := binary.LittleEndian.Uint32(msg.Data[:4])
		if int(id) >= total {
			t.Fatalf("Recv() id = %d, want < %d", id, total)
		}
		if seen[id] {
			t.Fatalf("Recv() duplicate id %d", id)
		}
		seen[id] = true
	}

	wg.Wait()
}
