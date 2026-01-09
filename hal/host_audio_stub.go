//go:build !tinygo && !cgo

package hal

// hostAudio is a stub when CGO/window backends are unavailable.
type hostAudio struct{}

func newHostAudio() hostAudio { return hostAudio{} }

func (a hostAudio) PWM() PWMAudio { return nil }
