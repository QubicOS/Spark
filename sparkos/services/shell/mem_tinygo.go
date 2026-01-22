//go:build tinygo

package shell

import (
	"fmt"
	"runtime"
)

func memStatusLine() string {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	// Alloc is bytes of allocated heap objects.
	// Sys is bytes obtained from the OS (or runtime) for heap and related structures.
	return fmt.Sprintf("mem: alloc=%d sys=%d\n", ms.Alloc, ms.Sys)
}
