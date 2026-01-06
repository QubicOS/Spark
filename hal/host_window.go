//go:build !tinygo

package hal

import (
	"image"
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
	fbImg   *ebiten.Image
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
		if g.fbImg != nil {
			g.fbImg.Deallocate()
		}
		g.fbImg = ebiten.NewImage(fb.width, fb.height)
	}

	fb.snapshotRGB565(g.scratch)

	src := g.scratch
	dst := g.img.Pix
	for i := 0; i+1 < len(src) && i/2*4+3 < len(dst); i += 2 {
		r, gg, b := rgb888From565(uint16(src[i]) | uint16(src[i+1])<<8)
		j := (i / 2) * 4
		dst[j+0] = r
		dst[j+1] = gg
		dst[j+2] = b
		dst[j+3] = 0xFF
	}

	g.fbImg.ReplacePixels(g.img.Pix)
	screen.DrawImage(g.fbImg, nil)
}

func (g *hostGame) Layout(outsideWidth, outsideHeight int) (int, int) {
	return g.h.fb.width, g.h.fb.height
}
