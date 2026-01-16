package quarkgl

// OrbitController provides basic orbit/zoom/pan interactions for a camera.
//
// It is intentionally simple and does not depend on any input system.
type OrbitController struct {
	Target Vec3
	Yaw    Scalar
	Pitch  Scalar
	Radius Scalar

	MinRadius Scalar
	MaxRadius Scalar
}

func (c *OrbitController) Apply(cam *Camera) {
	if cam == nil {
		return
	}
	r := c.Radius
	if r == 0 {
		r = Scalar(3)
	}
	if c.MinRadius != 0 && r < c.MinRadius {
		r = c.MinRadius
	}
	if c.MaxRadius != 0 && r > c.MaxRadius {
		r = c.MaxRadius
	}

	// Build a position from yaw/pitch (float backend uses real trig; fixed backend
	// uses identity rotations, so this degenerates but stays safe).
	m := Mat4Mul(Mat4RotateY(c.Yaw), Mat4RotateX(c.Pitch))
	p := Mat4MulV4(m, Vec4{X: 0, Y: 0, Z: r, W: 1})

	cam.Position = c.Target.Add(V3(p.X, p.Y, p.Z))
	cam.Target = c.Target
	if cam.Up == (Vec3{}) {
		cam.Up = V3(0, 1, 0)
	}
}

func (c *OrbitController) Rotate(deltaYaw, deltaPitch Scalar) {
	c.Yaw += deltaYaw
	c.Pitch += deltaPitch
}

func (c *OrbitController) Zoom(delta Scalar) {
	c.Radius += delta
	if c.MinRadius != 0 && c.Radius < c.MinRadius {
		c.Radius = c.MinRadius
	}
	if c.MaxRadius != 0 && c.Radius > c.MaxRadius {
		c.Radius = c.MaxRadius
	}
}
