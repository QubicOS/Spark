//go:build !tinygo

package kernel

import "runtime/debug"

func captureStack() []byte {
	return debug.Stack()
}
