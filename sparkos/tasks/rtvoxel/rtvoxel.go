package rtvoxel

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

	small   []rgb
	camDirs []vec3

	frame       uint64
	lastDrawSeq uint64

	pos   vec3
	yaw   float32
	pitch float32

	light vec3
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

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}
			switch proto.Kind(msg.Kind) {
			case proto.MsgAppShutdown:
				t.setActive(false)
				return

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

		case 'w':
			t.move(0.35, 0)
		case 's':
			t.move(-0.35, 0)
		case 'a':
			t.move(0, -0.35)
		case 'd':
			t.move(0, 0.35)
		case 'e':
			t.pos.y += 0.35
		case 'c':
			t.pos.y -= 0.35

		case 'j':
			t.yaw -= 0.12
		case 'l':
			t.yaw += 0.12
		case 'i':
			t.pitch += 0.08
		case 'k':
			t.pitch -= 0.08
		case 'r':
			t.resetView()
		}
	}
}

func (t *Task) move(forward, right float32) {
	sin, cos := sincos(t.yaw)
	t.pos.x += cos*forward + sin*right
	t.pos.z += sin*forward - cos*right
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
	if w != t.width || h != t.height || t.small == nil || t.camDirs == nil {
		t.width = w
		t.height = h
		// Raymarching is significantly heavier than the sphere RT demo, so render
		// at a lower internal resolution to keep frame times reasonable.
		t.sW = w / 4
		t.sH = h / 4
		if t.sW <= 0 || t.sH <= 0 {
			t.active = false
			return
		}
		t.small = make([]rgb, t.sW*t.sH)
		t.camDirs = make([]vec3, t.sW*t.sH)
	}

	aspect := float32(t.sW) / float32(t.sH)
	fovScale := float32(0.9)
	for y := 0; y < t.sH; y++ {
		for x := 0; x < t.sW; x++ {
			nx := (float32(x)+0.5)/float32(t.sW)*2 - 1
			ny := 1 - (float32(y)+0.5)/float32(t.sH)*2
			v := vec3{
				x: nx * aspect * fovScale,
				y: ny * fovScale,
				z: 1,
			}.normalize()
			t.camDirs[y*t.sW+x] = v
		}
	}

	t.resetView()
	t.light = vec3{0.4, 0.9, -0.2}.normalize()
}

func (t *Task) resetView() {
	t.pos = vec3{0.5, 8, -10.5}
	t.yaw = 0
	t.pitch = -0.10
}

func (t *Task) renderFrame() {
	w := t.width
	h := t.height
	if w <= 0 || h <= 0 || t.sW <= 0 || t.sH <= 0 {
		return
	}

	t.pos.y = clampF32(t.pos.y, 1, 30)
	t.pitch = clampF32(t.pitch, -0.8, 0.8)

	tt := float32(t.frame) * 0.03
	sunYaw := tt * 0.15
	sinSun, cosSun := sincos(sunYaw)
	t.light = vec3{cosSun * 0.6, 0.9, sinSun * 0.6}.normalize()

	forward, right, up := cameraBasis(t.yaw, t.pitch)

	for y := 0; y < t.sH; y++ {
		for x := 0; x < t.sW; x++ {
			dc := t.camDirs[y*t.sW+x]
			dir := right.scale(dc.x).add(up.scale(dc.y)).add(forward.scale(dc.z)).normalize()
			col := shadeRay(t.pos, dir, t.light)
			t.small[y*t.sW+x] = col
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
		sy := (y * t.sH) / h
		if sy < 0 {
			sy = 0
		} else if sy >= t.sH {
			sy = t.sH - 1
		}
		row := y * stride
		for x := 0; x < w; x++ {
			sx := (x * t.sW) / w
			if sx < 0 {
				sx = 0
			} else if sx >= t.sW {
				sx = t.sW - 1
			}
			c := t.small[sy*t.sW+sx]

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

func rgb565(r, g, b uint8) uint16 {
	return uint16((uint16(r>>3)&0x1F)<<11 | (uint16(g>>2)&0x3F)<<5 | (uint16(b>>3) & 0x1F))
}

type vec3 struct {
	x, y, z float32
}

func (v vec3) add(o vec3) vec3      { return vec3{v.x + o.x, v.y + o.y, v.z + o.z} }
func (v vec3) scale(s float32) vec3 { return vec3{v.x * s, v.y * s, v.z * s} }

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

func clampF32(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func sincos(x float32) (sin, cos float32) {
	sin = float32(math.Sin(float64(x)))
	cos = float32(math.Cos(float64(x)))
	return sin, cos
}

func cameraBasis(yaw, pitch float32) (forward, right, up vec3) {
	sinYaw, cosYaw := sincos(yaw)
	sinPitch, cosPitch := sincos(pitch)

	forward = vec3{sinYaw * cosPitch, sinPitch, cosYaw * cosPitch}.normalize()
	right = vec3{cosYaw, 0, -sinYaw}.normalize()
	up = vec3{-sinYaw * sinPitch, cosPitch, -cosYaw * sinPitch}.normalize()
	return forward, right, up
}

type block uint8

const (
	blockAir block = iota
	blockGrass
	blockDirt
	blockStone
	blockWater
)

func shadeRay(origin, dir, light vec3) rgb {
	const (
		maxDist  = 32
		maxSteps = 64
	)

	dist, normal, b, hit := cast(origin, dir, maxDist, maxSteps)
	sky := skyColor(dir)
	if !hit {
		return sky
	}

	base := blockColor(b)
	ndotl := normal.dot(light)
	if ndotl < 0 {
		ndotl = 0
	}
	faceShade := float32(0.15)
	if normal.x != 0 {
		faceShade = 0.10
	} else if normal.z != 0 {
		faceShade = 0.05
	}

	intensity := clampF32(0.18+0.82*ndotl-faceShade, 0, 1)
	col := vec3{
		x: base.x * intensity,
		y: base.y * intensity,
		z: base.z * intensity,
	}

	fog := clampF32(dist/float32(maxDist), 0, 1)
	col = col.scale(1 - fog).add(vec3{float32(sky.r), float32(sky.g), float32(sky.b)}.scale(fog))

	return rgb{
		r: uint8(clampF32(col.x, 0, 255)),
		g: uint8(clampF32(col.y, 0, 255)),
		b: uint8(clampF32(col.z, 0, 255)),
	}
}

func skyColor(dir vec3) rgb {
	t := clampF32(dir.y*0.5+0.5, 0, 1)
	r := 20 + 60*t
	g := 30 + 90*t
	b := 80 + 150*t
	return rgb{r: uint8(r), g: uint8(g), b: uint8(b)}
}

func blockColor(b block) vec3 {
	switch b {
	case blockGrass:
		return vec3{40, 170, 60}
	case blockDirt:
		return vec3{120, 80, 45}
	case blockStone:
		return vec3{125, 125, 135}
	case blockWater:
		return vec3{20, 80, 160}
	default:
		return vec3{255, 0, 255}
	}
}

func cast(origin, dir vec3, maxDist float32, maxSteps int) (dist float32, normal vec3, b block, hit bool) {
	cellX := fastFloor(origin.x)
	cellY := fastFloor(origin.y)
	cellZ := fastFloor(origin.z)

	stepX, tMaxX, tDeltaX := ddaInit(origin.x, dir.x, cellX)
	stepY, tMaxY, tDeltaY := ddaInit(origin.y, dir.y, cellY)
	stepZ, tMaxZ, tDeltaZ := ddaInit(origin.z, dir.z, cellZ)

	if bb := blockAt(cellX, cellY, cellZ); bb != blockAir {
		return 0, vec3{0, 1, 0}, bb, true
	}

	for range maxSteps {
		if tMaxX < tMaxY {
			if tMaxX < tMaxZ {
				cellX += stepX
				dist = tMaxX
				tMaxX += tDeltaX
				normal = vec3{float32(-stepX), 0, 0}
			} else {
				cellZ += stepZ
				dist = tMaxZ
				tMaxZ += tDeltaZ
				normal = vec3{0, 0, float32(-stepZ)}
			}
		} else {
			if tMaxY < tMaxZ {
				cellY += stepY
				dist = tMaxY
				tMaxY += tDeltaY
				normal = vec3{0, float32(-stepY), 0}
			} else {
				cellZ += stepZ
				dist = tMaxZ
				tMaxZ += tDeltaZ
				normal = vec3{0, 0, float32(-stepZ)}
			}
		}

		if dist > maxDist {
			return 0, vec3{}, 0, false
		}

		if bb := blockAt(cellX, cellY, cellZ); bb != blockAir {
			return dist, normal, bb, true
		}
	}

	return 0, vec3{}, 0, false
}

func ddaInit(pos, dir float32, cell int) (step int, tMax, tDelta float32) {
	if dir > 0 {
		step = 1
		next := float32(cell+1) - pos
		tMax = next / dir
		tDelta = 1 / dir
		return step, tMax, tDelta
	}
	if dir < 0 {
		step = -1
		next := pos - float32(cell)
		ndir := -dir
		tMax = next / ndir
		tDelta = 1 / ndir
		return step, tMax, tDelta
	}
	return 0, float32(math.Inf(1)), float32(math.Inf(1))
}

func fastFloor(v float32) int {
	i := int(v)
	if v < 0 && float32(i) != v {
		return i - 1
	}
	return i
}

func blockAt(x, y, z int) block {
	if y < 0 {
		return blockStone
	}
	if y > 31 {
		return blockAir
	}

	h := terrainHeight(x, z)
	waterLevel := 4
	if y > h {
		if y <= waterLevel {
			return blockWater
		}
		return blockAir
	}

	if y == h {
		return blockGrass
	}
	if y >= h-2 {
		return blockDirt
	}
	return blockStone
}

func terrainHeight(x, z int) int {
	n := hash2(uint32(x), uint32(z))
	h := 5 + int((n>>29)&0x7)
	if x == 0 && z == 0 {
		h = 7
	}
	return h
}

func hash2(x, z uint32) uint32 {
	// Low-cost mixing; stable across architectures.
	h := x*0x9e3779b1 ^ z*0x85ebca6b
	h ^= h >> 16
	h *= 0x7feb352d
	h ^= h >> 15
	h *= 0x846ca68b
	h ^= h >> 16
	return h
}
