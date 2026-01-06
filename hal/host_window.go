//go:build !tinygo

package hal

import (
	"image"
	"image/color"
	"spark/internal/buildinfo"

	"github.com/hajimehoshi/ebiten/v2"
)

// RunWindow starts a desktop window that displays the framebuffer and forwards keyboard input.
// It blocks until the window closes.
func RunWindow(newApp func(HAL) func() error) error {
	h := New().(*hostHAL)
	step := newApp(h)

	g := &hostGame{h: h, step: step}
	ebiten.SetWindowTitle("Spark (" + buildinfo.Short() + ")")
	ebiten.SetWindowSize(h.fb.width*2, h.fb.height*2)
	ebiten.SetTPS(60)
	return ebiten.RunGame(g)
}

type hostGame struct {
	h       *hostHAL
	img     *image.RGBA
	scratch []byte
	step    func() error
}

func (g *hostGame) Update() error {
	g.h.kbd.poll()
	g.h.t.step(1)
	if g.step != nil {
		if err := g.step(); err != nil {
			return err
		}
	}
	return nil
}

func (g *hostGame) Draw(screen *ebiten.Image) {
	fb := g.h.fb
	if g.img == nil || g.img.Bounds().Dx() != fb.width || g.img.Bounds().Dy() != fb.height {
		g.img = image.NewRGBA(image.Rect(0, 0, fb.width, fb.height))
		g.scratch = make([]byte, len(fb.buf))
	}

	fb.snapshotRGB565(g.scratch)

	p := 0
	for y := 0; y < fb.height; y++ {
		for x := 0; x < fb.width; x++ {
			r, gg, b := rgb888From565(uint16(g.scratch[p]) | uint16(g.scratch[p+1])<<8)
			g.img.SetRGBA(x, y, color.RGBA{R: r, G: gg, B: b, A: 0xFF})
			p += 2
		}
	}

	screen.ReplacePixels(g.img.Pix)
}

func (g *hostGame) Layout(outsideWidth, outsideHeight int) (int, int) {
	return g.h.fb.width, g.h.fb.height
}
