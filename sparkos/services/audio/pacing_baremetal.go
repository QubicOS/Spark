//go:build tinygo && baremetal

package audio

func needsSamplePacing() bool { return true }
