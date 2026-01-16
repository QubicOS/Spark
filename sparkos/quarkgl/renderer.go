package quarkgl

// Renderer is a fixed-pipeline software renderer.
//
// Create it once and reuse it to avoid allocations.
type Renderer struct {
	Mode       RenderMode
	Depth      bool
	ClearColor Color

	depthBuf []float32
}

// NewRenderer creates a renderer for a given maximum target size.
//
// If enableDepth is true, a depth buffer of size w*h is allocated.
func NewRenderer(w, h int, enableDepth bool) *Renderer {
	r := &Renderer{
		Mode:       RenderSolidFlat,
		Depth:      enableDepth,
		ClearColor: RGB(0, 0, 0),
	}
	if enableDepth && w > 0 && h > 0 {
		r.depthBuf = make([]float32, w*h)
	}
	return r
}

func (r *Renderer) SetRenderMode(m RenderMode) { r.Mode = m }

func (r *Renderer) EnableDepth(on bool, w, h int) {
	r.Depth = on
	if !on {
		r.depthBuf = nil
		return
	}
	if w <= 0 || h <= 0 {
		r.depthBuf = nil
		return
	}
	if cap(r.depthBuf) < w*h {
		r.depthBuf = make([]float32, w*h)
	} else {
		r.depthBuf = r.depthBuf[:w*h]
	}
}

func (r *Renderer) clearDepth() {
	for i := range r.depthBuf {
		r.depthBuf[i] = 1e9
	}
}

// Render renders a scene into the target.
func (r *Renderer) Render(t Target, s *Scene) {
	if r == nil || t == nil || s == nil {
		return
	}
	w, h := t.Size()
	if w <= 0 || h <= 0 {
		return
	}
	t.Clear(r.ClearColor)

	if r.Depth {
		r.EnableDepth(true, w, h)
		r.clearDepth()
	}

	aspect := Scalar(1)
	if h != 0 {
		aspect = scalarFromF32(float32(w) / float32(h))
	}
	view := s.Camera.View()
	proj := s.Camera.Projection(aspect)

	s.eachMesh(func(m *Mesh) {
		if m == nil || !m.Enabled {
			return
		}
		r.renderMesh(t, w, h, proj, view, *m, s.Light)
	})
}

func (r *Renderer) renderMesh(t Target, w, h int, proj, view Mat4, m Mesh, light Light) {
	if len(m.Vertices) == 0 || len(m.Indices) < 3 {
		return
	}
	if m.Transform == (Mat4{}) {
		m.Transform = Mat4Identity()
	}

	mvp := Mat4Mul(proj, Mat4Mul(view, m.Transform))

	for i := 0; i+2 < len(m.Indices); i += 3 {
		i0 := int(m.Indices[i+0])
		i1 := int(m.Indices[i+1])
		i2 := int(m.Indices[i+2])
		if i0 < 0 || i1 < 0 || i2 < 0 || i0 >= len(m.Vertices) || i1 >= len(m.Vertices) || i2 >= len(m.Vertices) {
			continue
		}

		v0 := m.Vertices[i0]
		v1 := m.Vertices[i1]
		v2 := m.Vertices[i2]

		p0 := Mat4MulV4(mvp, Vec4{X: v0.Pos.X, Y: v0.Pos.Y, Z: v0.Pos.Z, W: 1})
		p1 := Mat4MulV4(mvp, Vec4{X: v1.Pos.X, Y: v1.Pos.Y, Z: v1.Pos.Z, W: 1})
		p2 := Mat4MulV4(mvp, Vec4{X: v2.Pos.X, Y: v2.Pos.Y, Z: v2.Pos.Z, W: 1})

		// Trivial clip: if any vertex is behind the near plane (w<=0), drop.
		if p0.W == 0 || p1.W == 0 || p2.W == 0 {
			continue
		}

		ndc0, ok0 := clipToNDC(p0)
		ndc1, ok1 := clipToNDC(p1)
		ndc2, ok2 := clipToNDC(p2)
		if !ok0 || !ok1 || !ok2 {
			continue
		}

		// Screen coords.
		x0, y0 := ndcToScreen(ndc0, w, h)
		x1, y1 := ndcToScreen(ndc1, w, h)
		x2, y2 := ndcToScreen(ndc2, w, h)

		base := m.Material.BaseColor
		if light.Mode == LightAmbientDirectional {
			n := triangleNormal(v0.Pos, v1.Pos, v2.Pos)
			intensity := lightIntensity(light, n)
			base = base.MulScalar(intensity)
		}

		switch r.Mode {
		case RenderWireframe:
			c := base
			r.drawLine(t, x0, y0, x1, y1, c)
			r.drawLine(t, x1, y1, x2, y2, c)
			r.drawLine(t, x2, y2, x0, y0, c)
		case RenderSolidVertexColor:
			r.fillTriangle(t, w, h, x0, y0, ndc0.Z, v0.Color, x1, y1, ndc1.Z, v1.Color, x2, y2, ndc2.Z, v2.Color)
		default:
			r.fillTriangleFlat(t, w, h, x0, y0, ndc0.Z, x1, y1, ndc1.Z, x2, y2, ndc2.Z, base)
		}
	}
}

type ndcPoint struct {
	X, Y, Z float32
}

func clipToNDC(p Vec4) (ndcPoint, bool) {
	w := scalarToF32(p.W)
	if w == 0 {
		return ndcPoint{}, false
	}
	invW := 1.0 / w
	return ndcPoint{
		X: scalarToF32(p.X) * float32(invW),
		Y: scalarToF32(p.Y) * float32(invW),
		Z: scalarToF32(p.Z) * float32(invW),
	}, true
}

func ndcToScreen(p ndcPoint, w, h int) (x, y int) {
	sx := (p.X*0.5 + 0.5) * float32(w-1)
	sy := (1 - (p.Y*0.5 + 0.5)) * float32(h-1)
	return int(sx + 0.5), int(sy + 0.5)
}

func triangleNormal(a, b, c Vec3) Vec3 {
	return Normalize(Cross(b.Sub(a), c.Sub(a)))
}

func lightIntensity(l Light, n Vec3) Scalar {
	amb := Clamp01(l.Ambient)
	dir := Clamp01(l.DirAmount)
	ld := Normalize(l.Dir)
	if ld == (Vec3{}) {
		return amb
	}
	d := Dot(n, ld.Mul(-1))
	if d < 0 {
		d = 0
	}
	return Clamp01(amb + d*dir)
}

func (r *Renderer) depthTest(w int, x, y int, z float32) bool {
	if !r.Depth || r.depthBuf == nil {
		return true
	}
	if x < 0 || y < 0 || x >= w {
		return false
	}
	idx := y*w + x
	if idx < 0 || idx >= len(r.depthBuf) {
		return false
	}
	// NDC z is typically in [-1,1]. Map to [0,1].
	d := (z*0.5 + 0.5)
	if d < 0 {
		d = 0
	}
	if d > 1 {
		d = 1
	}
	if d >= r.depthBuf[idx] {
		return false
	}
	r.depthBuf[idx] = d
	return true
}

func (r *Renderer) drawLine(t Target, x0, y0, x1, y1 int, c Color) {
	dx := absInt(x1 - x0)
	sx := -1
	if x0 < x1 {
		sx = 1
	}
	dy := -absInt(y1 - y0)
	sy := -1
	if y0 < y1 {
		sy = 1
	}
	err := dx + dy
	for {
		t.SetPixel(x0, y0, c)
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

func (r *Renderer) fillTriangleFlat(t Target, w, h int, x0, y0 int, z0 float32, x1, y1 int, z1 float32, x2, y2 int, z2 float32, c Color) {
	minX, maxX := min3(x0, x1, x2), max3(x0, x1, x2)
	minY, maxY := min3(y0, y1, y2), max3(y0, y1, y2)
	if minX < 0 {
		minX = 0
	}
	if minY < 0 {
		minY = 0
	}
	if maxX >= w {
		maxX = w - 1
	}
	if maxY >= h {
		maxY = h - 1
	}
	if minX > maxX || minY > maxY {
		return
	}

	area := edgeFn(x0, y0, x1, y1, x2, y2)
	if area == 0 {
		return
	}
	invArea := 1.0 / float32(area)

	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			w0 := edgeFn(x1, y1, x2, y2, x, y)
			w1 := edgeFn(x2, y2, x0, y0, x, y)
			w2 := edgeFn(x0, y0, x1, y1, x, y)
			if (w0 | w1 | w2) < 0 {
				continue
			}
			a0 := float32(w0) * invArea
			a1 := float32(w1) * invArea
			a2 := float32(w2) * invArea
			z := a0*z0 + a1*z1 + a2*z2
			if !r.depthTest(w, x, y, z) {
				continue
			}
			t.SetPixel(x, y, c)
		}
	}
}

func (r *Renderer) fillTriangle(t Target, w, h int, x0, y0 int, z0 float32, c0 Color, x1, y1 int, z1 float32, c1 Color, x2, y2 int, z2 float32, c2 Color) {
	minX, maxX := min3(x0, x1, x2), max3(x0, x1, x2)
	minY, maxY := min3(y0, y1, y2), max3(y0, y1, y2)
	if minX < 0 {
		minX = 0
	}
	if minY < 0 {
		minY = 0
	}
	if maxX >= w {
		maxX = w - 1
	}
	if maxY >= h {
		maxY = h - 1
	}
	if minX > maxX || minY > maxY {
		return
	}

	area := edgeFn(x0, y0, x1, y1, x2, y2)
	if area == 0 {
		return
	}
	invArea := 1.0 / float32(area)

	r0, g0, b0 := float32(c0.R), float32(c0.G), float32(c0.B)
	r1, g1, b1 := float32(c1.R), float32(c1.G), float32(c1.B)
	r2, g2, b2 := float32(c2.R), float32(c2.G), float32(c2.B)

	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			w0 := edgeFn(x1, y1, x2, y2, x, y)
			w1 := edgeFn(x2, y2, x0, y0, x, y)
			w2 := edgeFn(x0, y0, x1, y1, x, y)
			if (w0 | w1 | w2) < 0 {
				continue
			}
			a0 := float32(w0) * invArea
			a1 := float32(w1) * invArea
			a2 := float32(w2) * invArea
			z := a0*z0 + a1*z1 + a2*z2
			if !r.depthTest(w, x, y, z) {
				continue
			}
			rr := uint8(clampF32(a0*r0+a1*r1+a2*r2, 0, 255))
			gg := uint8(clampF32(a0*g0+a1*g1+a2*g2, 0, 255))
			bb := uint8(clampF32(a0*b0+a1*b1+a2*b2, 0, 255))
			t.SetPixel(x, y, Color{R: rr, G: gg, B: bb, A: 0xFF})
		}
	}
}

func edgeFn(x0, y0, x1, y1, x, y int) int {
	return (x-x0)*(y1-y0) - (y-y0)*(x1-x0)
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func min3(a, b, c int) int {
	if a > b {
		a = b
	}
	if a > c {
		a = c
	}
	return a
}

func max3(a, b, c int) int {
	if a < b {
		a = b
	}
	if a < c {
		a = c
	}
	return a
}

func clampF32(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
