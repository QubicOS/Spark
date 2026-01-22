//go:build tinygo && bootdebug

package app

import (
	"machine"
	"sync"
	"time"

	"spark/hal"
)

var (
	bootDiagMu   sync.Mutex
	bootDiagStep string
)

func bootDiagSetStep(msg string) {
	bootDiagMu.Lock()
	bootDiagStep = msg
	bootDiagMu.Unlock()
}

func bootDiagStart(h hal.HAL) {
	if h == nil {
		return
	}
	l := h.Logger()

	go func() {
		for {
			bootDiagMu.Lock()
			step := bootDiagStep
			bootDiagMu.Unlock()

			if step == "" {
				step = "<empty>"
			}
			line := "bootdiag: " + step

			if l != nil {
				l.WriteLineString(line)
			}

			// Also stream to USB CDC when it becomes available.
			// This lets you "catch" early boot info without a separate UART adapter.
			if usb := machine.USBCDC; usb != nil {
				_, _ = usb.Write([]byte(line + "\r\n"))
			}

			time.Sleep(250 * time.Millisecond)
		}
	}()
}
