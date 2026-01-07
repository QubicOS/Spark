//go:build tinygo && !baremetal

package hal

func newTinyGoAudio() Audio { return nullAudio{} }
