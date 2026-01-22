package hal

import (
	"testing"
	"time"
)

func TestSignalPinRead(t *testing.T) {
	now := time.Unix(0, 0)
	clock := func() time.Time { return now }

	pin := newSignalPinWithClock("SIG", 10*time.Second, 2*time.Second, clock)
	if pin == nil {
		t.Fatal("expected pin")
	}

	level, err := pin.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !level {
		t.Fatal("expected high at t=0")
	}

	now = now.Add(3 * time.Second)
	level, err = pin.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if level {
		t.Fatal("expected low at t=3s")
	}

	now = now.Add(8 * time.Second) // t=11s => phase 1s, high again
	level, err = pin.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !level {
		t.Fatal("expected high at t=11s")
	}
}
