package quarkdonut

import (
	"image/color"
	"math"

	"spark/hal"
	"spark/sparkos/fonts/font6x8cp1251"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"
	"spark/sparkos/quarkgl"

	"tinygo.org/x/tinyfont"
)

type Task struct {
	disp hal.Display
	ep   kernel.Capability

	fb hal.Framebuffer

	font       tinyfont.Fonter
	fontWidth  int16
	fontHeight int16

	active bool
	muxCap kernel.Capability

	w int
	h int

	r *quarkgl.Renderer
	s *quarkgl.Scene

	meshID int

	angleY quarkgl.Scalar

	inbuf []byte
}

const frameIntervalTicks = 33

func New(disp hal.Display, ep kernel.Capability) *Task {
	return &Task{
		disp:   disp,
		ep:     ep,
		meshID: -1,
	}
}

func (t *Task) Run(ctx *kernel.Context) {
	ch, ok := ctx.RecvChan(t.ep)
	if !ok {
		return
	}
	if t.disp == nil {
		return
	}

	t.fb = t.disp.Framebuffer()
	if t.fb == nil || t.fb.Format() != hal.PixelFormatRGB565 {
		return
	}
	if !t.initFont() {
		return
	}

	t.w = t.fb.Width()
	t.h = t.fb.Height()
	if t.w <= 0 || t.h <= 0 {
		return
	}

	done := make(chan struct{})
	defer close(done)

	tickCh := make(chan uint64, 16)
	go func() {
		last := ctx.NowTick()
		for {
			select {
			case <-done:
				return
			default:
			}
			last = ctx.WaitTick(last)
			select {
			case tickCh <- last:
			default:
			}
		}
	}()

	var lastFrame uint64

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}
			switch proto.Kind(msg.Kind) {
			case proto.MsgAppShutdown:
				t.unload()
				return

			case proto.MsgAppControl:
				if msg.Cap.Valid() {
					t.muxCap = msg.Cap
				}
				active, ok := proto.DecodeAppControlPayload(msg.Payload())
				if !ok {
					continue
				}
				t.setActive(active)
				if t.active {
					t.render()
				}

			case proto.MsgAppSelect:
				appID, _, ok := proto.DecodeAppSelectPayload(msg.Payload())
				if !ok || appID != proto.AppQuarkDonut {
					continue
				}
				if t.active {
					t.render()
				}

			case proto.MsgTermInput:
				if !t.active {
					continue
				}
				t.handleInput(ctx, msg.Payload())
				if t.active {
					t.render()
				}
			}

		case now := <-tickCh:
			if !t.active {
				continue
			}
			if lastFrame != 0 && now-lastFrame < frameIntervalTicks {
				continue
			}
			lastFrame = now
			t.step()
			t.render()
		}
	}
}

func (t *Task) initFont() bool {
	t.font = font6x8cp1251.Font
	t.fontHeight = 8
	_, outboxWidth := tinyfont.LineWidth(t.font, "0")
	t.fontWidth = int16(outboxWidth)
	return t.fontWidth > 0 && t.fontHeight > 0
}

func (t *Task) ensureScene() {
	if t.s != nil && t.r != nil {
		return
	}

	t.r = quarkgl.NewRenderer(t.w, t.h, true)
	t.r.SetWorkers(2)
	t.r.ClearColor = quarkgl.RGB(0x05, 0x08, 0x12)
	t.r.Mode = quarkgl.RenderSolidFlat

	t.s = quarkgl.CreateScene(1)
	t.s.Camera.Type = quarkgl.CameraPerspective
	t.s.Camera.Position = quarkgl.V3(quarkgl.ScalarFromFloat32(0), quarkgl.ScalarFromFloat32(0.2), quarkgl.ScalarFromFloat32(3.2))
	t.s.Camera.Target = quarkgl.V3(0, 0, 0)
	t.s.Camera.Up = quarkgl.V3(0, quarkgl.ScalarFromFloat32(1), 0)
	t.s.Camera.FOVYRad = quarkgl.ScalarFromFloat32(1.0)
	t.s.Camera.Near = quarkgl.ScalarFromFloat32(0.05)
	t.s.Camera.Far = quarkgl.ScalarFromFloat32(20)

	t.s.Light.Mode = quarkgl.LightAmbientDirectional
	t.s.Light.Ambient = quarkgl.ScalarFromFloat32(0.18)
	t.s.Light.Dir = quarkgl.Normalize(quarkgl.V3(quarkgl.ScalarFromFloat32(-0.4), quarkgl.ScalarFromFloat32(0.9), quarkgl.ScalarFromFloat32(0.3)))
	t.s.Light.DirAmount = quarkgl.ScalarFromFloat32(0.85)

	mesh := newTorusMesh(1.0, 0.38, 32, 16)
	mesh.Material.BaseColor = quarkgl.RGB(0xFF, 0x99, 0x33)
	t.meshID = t.s.AddMesh(mesh)
}

func (t *Task) setActive(active bool) {
	if active == t.active {
		return
	}
	t.active = active
	if !t.active {
		return
	}
	t.ensureScene()
}

func (t *Task) unload() {
	t.active = false
	t.inbuf = nil
	t.r = nil
	t.s = nil
	t.meshID = -1
}

func (t *Task) requestExit(ctx *kernel.Context) {
	t.active = false
	if !t.muxCap.Valid() {
		return
	}
	_ = ctx.SendToCapRetry(t.muxCap, uint16(proto.MsgAppControl), proto.AppControlPayload(false), kernel.Capability{}, 500)
}

func (t *Task) handleInput(ctx *kernel.Context, b []byte) {
	for _, c := range b {
		switch c {
		case 0x1b, 'q':
			t.requestExit(ctx)
			return
		case 'w':
			if t.r == nil {
				continue
			}
			if t.r.Mode == quarkgl.RenderWireframe {
				t.r.Mode = quarkgl.RenderSolidFlat
			} else {
				t.r.Mode = quarkgl.RenderWireframe
			}
		}
	}
}

func (t *Task) step() {
	t.ensureScene()
	if t.s == nil || t.meshID < 0 {
		return
	}

	t.angleY += quarkgl.ScalarFromFloat32(0.08)
	tiltX := quarkgl.ScalarFromFloat32(0.65)
	model := quarkgl.Mat4Mul(quarkgl.Mat4RotateY(t.angleY), quarkgl.Mat4RotateX(tiltX))
	t.s.UpdateMeshTransform(t.meshID, model)
}

func (t *Task) render() {
	t.ensureScene()
	if t.fb == nil || t.r == nil || t.s == nil {
		return
	}

	target := &quarkgl.RGB565Target{
		Buf:    t.fb.Buffer(),
		Stride: t.fb.StrideBytes(),
		W:      t.w,
		H:      t.h,
	}

	t.r.Render(target, t.s)

	t.drawText(6, 6, "QuarkGL donut", color.RGBA{R: 0xE0, G: 0xE8, B: 0xFF, A: 0xFF})
	t.drawText(6, 16, "q/ESC exit   w wireframe", color.RGBA{R: 0x90, G: 0xA0, B: 0xB8, A: 0xFF})

	_ = t.fb.Present()
}

func (t *Task) drawText(x, y int, s string, c color.RGBA) {
	if t.fb == nil || t.fb.Format() != hal.PixelFormatRGB565 {
		return
	}
	d := &fbDisplayer{fb: t.fb}
	tinyfont.WriteLine(d, t.font, int16(x), int16(y)+t.fontHeight, s, c)
}

type fbDisplayer struct {
	fb hal.Framebuffer
}

func (d *fbDisplayer) Size() (x, y int16) {
	if d.fb == nil {
		return 0, 0
	}
	return int16(d.fb.Width()), int16(d.fb.Height())
}

func (d *fbDisplayer) SetPixel(x, y int16, c color.RGBA) {
	if d.fb == nil || d.fb.Format() != hal.PixelFormatRGB565 {
		return
	}
	buf := d.fb.Buffer()
	if buf == nil {
		return
	}

	w := d.fb.Width()
	h := d.fb.Height()
	ix := int(x)
	iy := int(y)
	if ix < 0 || ix >= w || iy < 0 || iy >= h {
		return
	}

	pixel := rgb565From888(c.R, c.G, c.B)
	off := iy*d.fb.StrideBytes() + ix*2
	if off < 0 || off+1 >= len(buf) {
		return
	}
	buf[off] = byte(pixel)
	buf[off+1] = byte(pixel >> 8)
}

func (d *fbDisplayer) Display() error { return nil }

func rgb565From888(r, g, b uint8) uint16 {
	return uint16((uint16(r>>3)&0x1F)<<11 | (uint16(g>>2)&0x3F)<<5 | (uint16(b>>3) & 0x1F))
}

func newTorusMesh(major, minor float32, segU, segV int) quarkgl.Mesh {
	if segU < 3 {
		segU = 3
	}
	if segV < 3 {
		segV = 3
	}

	verts := make([]quarkgl.Vertex, 0, segU*segV)
	indices := make([]uint16, 0, segU*segV*6)

	twoPi := float32(2 * math.Pi)
	for u := 0; u < segU; u++ {
		theta := twoPi * float32(u) / float32(segU)
		ct := float32(math.Cos(float64(theta)))
		st := float32(math.Sin(float64(theta)))
		for v := 0; v < segV; v++ {
			phi := twoPi * float32(v) / float32(segV)
			cp := float32(math.Cos(float64(phi)))
			sp := float32(math.Sin(float64(phi)))

			r := major + minor*cp
			x := r * ct
			y := minor * sp
			z := r * st

			verts = append(verts, quarkgl.Vertex{
				Pos: quarkgl.V3(
					quarkgl.ScalarFromFloat32(x),
					quarkgl.ScalarFromFloat32(y),
					quarkgl.ScalarFromFloat32(z),
				),
			})
		}
	}

	idx := func(u, v int) uint16 {
		uu := u % segU
		vv := v % segV
		return uint16(uu*segV + vv)
	}

	for u := 0; u < segU; u++ {
		for v := 0; v < segV; v++ {
			i0 := idx(u, v)
			i1 := idx(u+1, v)
			i2 := idx(u+1, v+1)
			i3 := idx(u, v+1)

			indices = append(indices, i0, i1, i2)
			indices = append(indices, i0, i2, i3)
		}
	}

	return quarkgl.Mesh{
		Vertices: verts,
		Indices:  indices,
	}
}
