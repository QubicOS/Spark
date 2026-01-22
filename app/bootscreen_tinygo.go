//go:build tinygo && bootdebug

package app

import (
	"image/color"

	"spark/hal"
	"spark/sparkos/fonts/dejavumono9"

	"tinygo.org/x/tinyfont"
)

func bootScreen(h hal.HAL, msg string) {
	bootDiagSetStep(msg)
	if h == nil {
		return
	}
	disp := h.Display()
	if disp == nil {
		return
	}
	fb := disp.Framebuffer()
	if fb == nil {
		return
	}

	fb.ClearRGB(0, 0, 0)

	d := panicDisplay{fb: fb}
	font := &dejavumono9.DejaVuSansMono9

	fg := color.RGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
	tinyfont.WriteLine(d, font, 0, 12, "Spark boot", fg)
	tinyfont.WriteLine(d, font, 0, 28, msg, fg)
	_ = fb.Present()
}
