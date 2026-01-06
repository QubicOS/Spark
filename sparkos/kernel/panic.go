package kernel

import (
	"sync"
	"sync/atomic"
)

// PanicInfo contains details about a recovered panic.
type PanicInfo struct {
	TaskID TaskID
	Value  any
	Stack  []byte
}

var (
	panicActive atomic.Bool
	panicOnce   sync.Once

	panicHandler atomic.Value // func(PanicInfo)
)

// InPanicMode reports whether the kernel is in panic mode.
func InPanicMode() bool {
	return panicActive.Load()
}

// SetPanicHandler installs a process-wide panic handler.
//
// The handler is invoked at most once (on the first panic). It must not panic.
func SetPanicHandler(fn func(PanicInfo)) {
	panicHandler.Store(fn)
}

func triggerPanic(info PanicInfo) {
	panicOnce.Do(func() {
		panicActive.Store(true)
		info.Stack = captureStack()
		if v := panicHandler.Load(); v != nil {
			if fn, ok := v.(func(PanicInfo)); ok && fn != nil {
				fn(info)
			}
		}
	})
}
