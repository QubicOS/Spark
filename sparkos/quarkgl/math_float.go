//go:build !quarkgl_fixed

package quarkgl

import "math"

// Scalar is the numeric type used by QuarkGL math operations.
//
// Default backend is float32. See the package docs for build tags.
type Scalar = float32

// Vec3 is a 3D vector.
type Vec3 struct {
	X, Y, Z Scalar
}

// Vec4 is a 4D vector.
type Vec4 struct {
	X, Y, Z, W Scalar
}

// Mat4 is a column-major 4x4 matrix.
//
// It matches the conventional OpenGL layout:
// m[col*4+row].
type Mat4 [16]Scalar

func V3(x, y, z Scalar) Vec3 { return Vec3{X: x, Y: y, Z: z} }

func (v Vec3) Add(o Vec3) Vec3   { return Vec3{v.X + o.X, v.Y + o.Y, v.Z + o.Z} }
func (v Vec3) Sub(o Vec3) Vec3   { return Vec3{v.X - o.X, v.Y - o.Y, v.Z - o.Z} }
func (v Vec3) Mul(s Scalar) Vec3 { return Vec3{v.X * s, v.Y * s, v.Z * s} }

func Dot(a, b Vec3) Scalar { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }

func Cross(a, b Vec3) Vec3 {
	return Vec3{
		X: a.Y*b.Z - a.Z*b.Y,
		Y: a.Z*b.X - a.X*b.Z,
		Z: a.X*b.Y - a.Y*b.X,
	}
}

func Len(v Vec3) Scalar {
	return Scalar(math.Sqrt(float64(Dot(v, v))))
}

func Normalize(v Vec3) Vec3 {
	l := Len(v)
	if l == 0 {
		return Vec3{}
	}
	return v.Mul(1 / l)
}

func Clamp01(v Scalar) Scalar {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func Mat4Identity() Mat4 {
	return Mat4{
		1, 0, 0, 0,
		0, 1, 0, 0,
		0, 0, 1, 0,
		0, 0, 0, 1,
	}
}

func Mat4Mul(a, b Mat4) Mat4 {
	var out Mat4
	for col := 0; col < 4; col++ {
		for row := 0; row < 4; row++ {
			out[col*4+row] =
				a[0*4+row]*b[col*4+0] +
					a[1*4+row]*b[col*4+1] +
					a[2*4+row]*b[col*4+2] +
					a[3*4+row]*b[col*4+3]
		}
	}
	return out
}

func Mat4MulV4(m Mat4, v Vec4) Vec4 {
	return Vec4{
		X: m[0]*v.X + m[4]*v.Y + m[8]*v.Z + m[12]*v.W,
		Y: m[1]*v.X + m[5]*v.Y + m[9]*v.Z + m[13]*v.W,
		Z: m[2]*v.X + m[6]*v.Y + m[10]*v.Z + m[14]*v.W,
		W: m[3]*v.X + m[7]*v.Y + m[11]*v.Z + m[15]*v.W,
	}
}

func Mat4Translate(v Vec3) Mat4 {
	m := Mat4Identity()
	m[12] = v.X
	m[13] = v.Y
	m[14] = v.Z
	return m
}

func Mat4Scale(v Vec3) Mat4 {
	m := Mat4Identity()
	m[0] = v.X
	m[5] = v.Y
	m[10] = v.Z
	return m
}

func Mat4RotateX(rad Scalar) Mat4 {
	c := Scalar(math.Cos(float64(rad)))
	s := Scalar(math.Sin(float64(rad)))
	return Mat4{
		1, 0, 0, 0,
		0, c, s, 0,
		0, -s, c, 0,
		0, 0, 0, 1,
	}
}

func Mat4RotateY(rad Scalar) Mat4 {
	c := Scalar(math.Cos(float64(rad)))
	s := Scalar(math.Sin(float64(rad)))
	return Mat4{
		c, 0, -s, 0,
		0, 1, 0, 0,
		s, 0, c, 0,
		0, 0, 0, 1,
	}
}

func Mat4RotateZ(rad Scalar) Mat4 {
	c := Scalar(math.Cos(float64(rad)))
	s := Scalar(math.Sin(float64(rad)))
	return Mat4{
		c, s, 0, 0,
		-s, c, 0, 0,
		0, 0, 1, 0,
		0, 0, 0, 1,
	}
}

func Mat4LookAt(eye, target, up Vec3) Mat4 {
	f := Normalize(target.Sub(eye))
	s := Normalize(Cross(f, up))
	u := Cross(s, f)

	// Column-major.
	return Mat4{
		s.X, u.X, -f.X, 0,
		s.Y, u.Y, -f.Y, 0,
		s.Z, u.Z, -f.Z, 0,
		-Dot(s, eye), -Dot(u, eye), Dot(f, eye), 1,
	}
}

func Mat4Perspective(fovYRad Scalar, aspect Scalar, zNear, zFar Scalar) Mat4 {
	if aspect == 0 {
		aspect = 1
	}
	f := Scalar(1) / Scalar(math.Tan(float64(fovYRad)/2))
	nf := Scalar(1) / (zNear - zFar)
	return Mat4{
		f / aspect, 0, 0, 0,
		0, f, 0, 0,
		0, 0, (zFar + zNear) * nf, -1,
		0, 0, (2 * zFar * zNear) * nf, 0,
	}
}

func Mat4Ortho(left, right, bottom, top, zNear, zFar Scalar) Mat4 {
	rl := right - left
	tb := top - bottom
	fn := zFar - zNear
	if rl == 0 {
		rl = 1
	}
	if tb == 0 {
		tb = 1
	}
	if fn == 0 {
		fn = 1
	}
	return Mat4{
		2 / rl, 0, 0, 0,
		0, 2 / tb, 0, 0,
		0, 0, -2 / fn, 0,
		-(right + left) / rl, -(top + bottom) / tb, -(zFar + zNear) / fn, 1,
	}
}

// ScalarFromFloat32 converts a float32 value to Scalar.
func ScalarFromFloat32(v float32) Scalar { return Scalar(v) }

// ScalarToFloat32 converts Scalar to float32.
func ScalarToFloat32(v Scalar) float32 { return float32(v) }

func scalarFromF32(v float32) Scalar { return ScalarFromFloat32(v) }
func scalarToF32(v Scalar) float32   { return ScalarToFloat32(v) }
