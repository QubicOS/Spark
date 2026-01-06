package rtdemo

import (
	"math"

	"spark/hal"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

type Task struct {
	disp hal.Display
	ep   kernel.Capability

	fb hal.Framebuffer

	active bool
	muxCap kernel.Capability

	width  int
	height int
	sW     int
	sH     int

	small []rgb
	dirs  []vec3

	frame       uint64
	lastDrawSeq uint64

	sphere  sphere
	sphere2 sphere
	checker checker
	objs    []rayObject

	light vec3
	cam   vec3
}

func New(disp hal.Display, ep kernel.Capability) *Task {
	return &Task{disp: disp, ep: ep}
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
	if t.fb == nil {
		return
	}

	tickCh := make(chan uint64, 16)
	go func() {
		last := ctx.NowTick()
		for {
			last = ctx.WaitTick(last)
			select {
			case tickCh <- last:
			default:
			}
		}
	}()

	for {
		select {
		case msg := <-ch:
			switch proto.Kind(msg.Kind) {
			case proto.MsgAppControl:
				if msg.Cap.Valid() {
					t.muxCap = msg.Cap
				}
				active, ok := proto.DecodeAppControlPayload(msg.Data[:msg.Len])
				if !ok {
					continue
				}
				t.setActive(active)
			case proto.MsgTermInput:
				if !t.active {
					continue
				}
				t.handleInput(ctx, msg.Data[:msg.Len])
			}

		case seq := <-tickCh:
			if !t.active {
				continue
			}
			if seq-t.lastDrawSeq < 2 {
				continue
			}
			t.lastDrawSeq = seq
			t.renderFrame()
		}
	}
}

func (t *Task) handleInput(ctx *kernel.Context, b []byte) {
	for _, c := range b {
		switch c {
		case 0x1b, 'q':
			t.requestExit(ctx)
			return
		}
	}
}

func (t *Task) requestExit(ctx *kernel.Context) {
	if !t.muxCap.Valid() {
		t.setActive(false)
		return
	}
	for {
		res := ctx.SendToCapResult(t.muxCap, uint16(proto.MsgAppControl), proto.AppControlPayload(false), kernel.Capability{})
		switch res {
		case kernel.SendOK:
			return
		case kernel.SendErrQueueFull:
			ctx.BlockOnTick()
		default:
			t.setActive(false)
			return
		}
	}
}

func (t *Task) setActive(active bool) {
	if active == t.active {
		return
	}
	t.active = active
	t.lastDrawSeq = 0
	t.frame = 0

	if !t.active {
		return
	}
	t.initScene()
}

func (t *Task) initScene() {
	w := t.fb.Width()
	h := t.fb.Height()
	if w <= 0 || h <= 0 {
		t.active = false
		return
	}
	if w != t.width || h != t.height || t.small == nil || t.dirs == nil {
		t.width = w
		t.height = h
		t.sW = w / 2
		t.sH = h / 2
		if t.sW <= 0 || t.sH <= 0 {
			t.active = false
			return
		}
		t.small = make([]rgb, t.sW*t.sH)
		t.dirs = make([]vec3, t.sW*t.sH)
	}

	invW := 1 / float32(t.width)
	for y := 0; y < t.sH; y++ {
		for x := 0; x < t.sW; x++ {
			v := vec3{
				x: float32(x*2-t.width/2) * invW,
				y: float32(t.height/2-y*2) * invW,
				z: 1,
			}
			v = v.normalize()
			t.dirs[y*t.sW+x] = v
		}
	}

	t.sphere = sphere{p: vec3{0, 0.5, 0}, r: 1, r2: 1, c: vec3{0, 1, 0}, reflection: 0.4}
	t.sphere2 = sphere{p: vec3{1, 1.5, 0.5}, r: 0.5, r2: 0.25, c: vec3{1, 0, 1}, reflection: 0.5}
	t.checker = checker{reflection: 0.2}
	t.objs = []rayObject{&t.sphere, &t.sphere2, &t.checker}

	t.light = vec3{5, 4, -5}.normalize()
	t.cam = vec3{0, 1, -10}
}

func (t *Task) renderFrame() {
	w := t.width
	h := t.height
	if w <= 0 || h <= 0 || t.sW <= 0 || t.sH <= 0 {
		return
	}

	tt := float32(t.frame) * 0.05
	t.sphere2.p.x = float32(math.Sin(float64(tt*0.5))) * 2
	t.sphere2.p.z = float32(math.Cos(float64(tt*0.5))) * 2
	t.sphere.p.y = t.sphere2.p.x*0.3 + 1

	for y := 0; y < t.sH; y++ {
		for x := 0; x < t.sW; x++ {
			r := ray{p: t.cam, d: t.dirs[y*t.sW+x]}
			col := raytrace(t.objs, r, t.light, 3, nil)
			t.small[y*t.sW+x] = rgb{
				r: clampU8(int(col.x * 255)),
				g: clampU8(int(col.y * 255)),
				b: clampU8(int(col.z * 255)),
			}
		}
	}

	if t.fb.Format() != hal.PixelFormatRGB565 {
		_ = t.fb.Present()
		return
	}
	buf := t.fb.Buffer()
	if buf == nil {
		return
	}
	stride := t.fb.StrideBytes()
	if stride <= 0 {
		return
	}

	for y := 0; y < h; y++ {
		sy := y >> 1
		sy1 := sy + 1
		if sy1 >= t.sH {
			sy1 = t.sH - 1
		}
		dy := (y & 1) != 0

		row := y * stride
		for x := 0; x < w; x++ {
			sx := x >> 1
			sx1 := sx + 1
			if sx1 >= t.sW {
				sx1 = t.sW - 1
			}
			dx := (x & 1) != 0

			c00 := t.small[sy*t.sW+sx]
			c := c00
			switch {
			case dx && !dy:
				c10 := t.small[sy*t.sW+sx1]
				c = rgb{
					r: uint8((int(c00.r) + int(c10.r)) / 2),
					g: uint8((int(c00.g) + int(c10.g)) / 2),
					b: uint8((int(c00.b) + int(c10.b)) / 2),
				}
			case !dx && dy:
				c01 := t.small[sy1*t.sW+sx]
				c = rgb{
					r: uint8((int(c00.r) + int(c01.r)) / 2),
					g: uint8((int(c00.g) + int(c01.g)) / 2),
					b: uint8((int(c00.b) + int(c01.b)) / 2),
				}
			case dx && dy:
				c10 := t.small[sy*t.sW+sx1]
				c01 := t.small[sy1*t.sW+sx]
				c11 := t.small[sy1*t.sW+sx1]
				c = rgb{
					r: uint8((int(c00.r) + int(c10.r) + int(c01.r) + int(c11.r)) / 4),
					g: uint8((int(c00.g) + int(c10.g) + int(c01.g) + int(c11.g)) / 4),
					b: uint8((int(c00.b) + int(c10.b) + int(c01.b) + int(c11.b)) / 4),
				}
			}

			pixel := rgb565(c.r, c.g, c.b)
			off := row + x*2
			if off < 0 || off+1 >= len(buf) {
				continue
			}
			buf[off] = byte(pixel)
			buf[off+1] = byte(pixel >> 8)
		}
	}

	_ = t.fb.Present()
	t.frame++
}

type rgb struct {
	r, g, b uint8
}

func clampU8(v int) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

func rgb565(r, g, b uint8) uint16 {
	return uint16((uint16(r>>3)&0x1F)<<11 | (uint16(g>>2)&0x3F)<<5 | (uint16(b>>3) & 0x1F))
}

type vec3 struct {
	x, y, z float32
}

func (v vec3) add(o vec3) vec3 { return vec3{v.x + o.x, v.y + o.y, v.z + o.z} }
func (v vec3) sub(o vec3) vec3 { return vec3{v.x - o.x, v.y - o.y, v.z - o.z} }
func (v vec3) scale(s float32) vec3 {
	return vec3{v.x * s, v.y * s, v.z * s}
}

func (v vec3) dot(o vec3) float32 { return v.x*o.x + v.y*o.y + v.z*o.z }

func (v vec3) normalize() vec3 {
	l2 := v.dot(v)
	if l2 == 0 {
		return v
	}
	return v.scale(rsqrt(l2))
}

func rsqrt(x float32) float32 {
	if x <= 0 {
		return 0
	}
	x2 := x * 0.5
	i := math.Float32bits(x)
	i = 0x5f3759df - (i >> 1)
	y := math.Float32frombits(i)
	y = y * (1.5 - x2*y*y)
	y = y * (1.5 - x2*y*y)
	return y
}

func sqrt(x float32) float32 {
	return x * rsqrt(x)
}

type ray struct {
	p vec3
	d vec3
}

type rayObject interface {
	intersect(ray ray, tMax float32) (hit bool, t float32, p vec3)
	normal(p vec3) vec3
	color(p vec3) vec3
	refl() float32
}

type sphere struct {
	reflection float32
	c          vec3

	r  float32
	r2 float32
	p  vec3
}

func (s *sphere) refl() float32 { return s.reflection }

func (s *sphere) intersect(ray ray, tMax float32) (bool, float32, vec3) {
	L := s.p.sub(ray.p)
	tca := L.dot(ray.d)
	if tca < 0 {
		return false, 0, vec3{}
	}
	d2 := L.dot(L) - tca*tca
	if d2 >= s.r2 {
		return false, 0, vec3{}
	}
	thc := sqrt(s.r2 - d2)
	ct := tca - thc
	if ct >= tMax {
		return false, 0, vec3{}
	}
	p := ray.p.add(ray.d.scale(ct))
	return true, ct, p
}

func (s *sphere) normal(p vec3) vec3 {
	return p.sub(s.p).scale(1 / s.r)
}

func (s *sphere) color(vec3) vec3 { return s.c }

type checker struct {
	reflection float32
}

func (c *checker) refl() float32 { return c.reflection }

func (c *checker) intersect(ray ray, tMax float32) (bool, float32, vec3) {
	if ray.d.y >= 0 || ray.p.y <= 0 {
		return false, 0, vec3{}
	}
	ct := ray.p.y / -ray.d.y
	if ct >= tMax {
		return false, 0, vec3{}
	}
	p := vec3{
		x: ray.p.x + ray.d.x*ct,
		y: 0,
		z: ray.p.z + ray.d.z*ct,
	}
	return true, ct, p
}

func (c *checker) normal(vec3) vec3 { return vec3{0, 1, 0} }

func (c *checker) color(p vec3) vec3 {
	ix := int(p.x)
	iz := int(p.z)
	if p.x >= 0 {
		ix++
	}
	cc := float32((ix + iz) & 1)
	return vec3{0.8 + 0.2*cc, cc, cc}
}

func raytrace(objs []rayObject, r ray, light vec3, depth int, self rayObject) vec3 {
	if depth == 0 {
		return vec3{0, 0, 0}
	}

	var (
		best  rayObject
		bestP vec3
		bestT float32 = 10000
	)
	for _, o := range objs {
		if o == self {
			continue
		}
		hit, tt, p := o.intersect(r, bestT)
		if !hit {
			continue
		}
		best = o
		bestT = tt
		bestP = p
	}

	fog := bestT * 0.02
	fc := float32(0.5)
	if r.d.y >= 0 {
		fc = 0.5 - r.d.y*0.5
	}
	fogc := vec3{fc, fc, 1}
	if fog >= 1 {
		return fogc
	}
	if best == nil {
		return fogc
	}

	n := best.normal(bestP)
	l := light.dot(n) * 0.9
	if l < 0 {
		l = 0
	} else {
		r2 := ray{p: bestP, d: light}
		t2 := float32(10000)
		for _, o := range objs {
			if o == best {
				continue
			}
			hit, _, _ := o.intersect(r2, t2)
			if hit {
				l = 0
				break
			}
		}
	}

	col := best.color(bestP).scale(0.1 + l)
	col = col.scale(1 - fog).add(fogc.scale(fog))
	if best.refl() == 0 {
		return col
	}

	dn := r.d.dot(n)
	fr := (0.2 + (1+dn)*0.8) * best.refl()
	if fr < 0 {
		fr = 0
	}

	refl := r.d.sub(n.scale(dn * 2))
	nr := ray{p: bestP, d: refl}
	reflCol := raytrace(objs, nr, light, depth-1, best)
	return reflCol.scale(fr).add(col.scale(1 - fr))
}
