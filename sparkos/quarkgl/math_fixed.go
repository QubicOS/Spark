//go:build quarkgl_fixed

package quarkgl

// Scalar is the numeric type used by QuarkGL math operations.
//
// Fixed backend uses Q16.16 signed fixed-point.
// NOTE: This backend is currently minimal and intended mainly to keep build
// compatibility for experiments. The float backend is recommended.
type Scalar = int32

const fixedShift = 16
const fixedOne Scalar = 1 << fixedShift

func fixedFromInt(v int32) Scalar { return v << fixedShift }

func fixedMul(a, b Scalar) Scalar { return Scalar((int64(a) * int64(b)) >> fixedShift) }

func fixedDiv(a, b Scalar) Scalar {
	if b == 0 {
		return 0
	}
	return Scalar((int64(a) << fixedShift) / int64(b))
}

// Vec3 is a 3D vector.
type Vec3 struct {
	X, Y, Z Scalar
}

// Vec4 is a 4D vector.
type Vec4 struct {
	X, Y, Z, W Scalar
}

// Mat4 is a column-major 4x4 matrix.
type Mat4 [16]Scalar

func V3(x, y, z Scalar) Vec3 { return Vec3{X: x, Y: y, Z: z} }

func (v Vec3) Add(o Vec3) Vec3   { return Vec3{v.X + o.X, v.Y + o.Y, v.Z + o.Z} }
func (v Vec3) Sub(o Vec3) Vec3   { return Vec3{v.X - o.X, v.Y - o.Y, v.Z - o.Z} }
func (v Vec3) Mul(s Scalar) Vec3 { return Vec3{fixedMul(v.X, s), fixedMul(v.Y, s), fixedMul(v.Z, s)} }

func Dot(a, b Vec3) Scalar {
	return fixedMul(a.X, b.X) + fixedMul(a.Y, b.Y) + fixedMul(a.Z, b.Z)
}

func Cross(a, b Vec3) Vec3 {
	return Vec3{
		X: fixedMul(a.Y, b.Z) - fixedMul(a.Z, b.Y),
		Y: fixedMul(a.Z, b.X) - fixedMul(a.X, b.Z),
		Z: fixedMul(a.X, b.Y) - fixedMul(a.Y, b.X),
	}
}

func Len(v Vec3) Scalar {
	// Stub: use squared length approximation (not a real length).
	return Dot(v, v)
}

func Normalize(v Vec3) Vec3 {
	// Stub: no-op normalization in fixed backend.
	return v
}

func Clamp01(v Scalar) Scalar {
	if v < 0 {
		return 0
	}
	if v > fixedOne {
		return fixedOne
	}
	return v
}

func Mat4Identity() Mat4 {
	return Mat4{
		fixedOne, 0, 0, 0,
		0, fixedOne, 0, 0,
		0, 0, fixedOne, 0,
		0, 0, 0, fixedOne,
	}
}

func Mat4Mul(a, b Mat4) Mat4 {
	var out Mat4
	for col := 0; col < 4; col++ {
		for row := 0; row < 4; row++ {
			out[col*4+row] =
				fixedMul(a[0*4+row], b[col*4+0]) +
					fixedMul(a[1*4+row], b[col*4+1]) +
					fixedMul(a[2*4+row], b[col*4+2]) +
					fixedMul(a[3*4+row], b[col*4+3])
		}
	}
	return out
}

func Mat4MulV4(m Mat4, v Vec4) Vec4 {
	return Vec4{
		X: fixedMul(m[0], v.X) + fixedMul(m[4], v.Y) + fixedMul(m[8], v.Z) + fixedMul(m[12], v.W),
		Y: fixedMul(m[1], v.X) + fixedMul(m[5], v.Y) + fixedMul(m[9], v.Z) + fixedMul(m[13], v.W),
		Z: fixedMul(m[2], v.X) + fixedMul(m[6], v.Y) + fixedMul(m[10], v.Z) + fixedMul(m[14], v.W),
		W: fixedMul(m[3], v.X) + fixedMul(m[7], v.Y) + fixedMul(m[11], v.Z) + fixedMul(m[15], v.W),
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

func Mat4RotateX(_ Scalar) Mat4 { return Mat4Identity() }
func Mat4RotateY(_ Scalar) Mat4 { return Mat4Identity() }
func Mat4RotateZ(_ Scalar) Mat4 { return Mat4Identity() }

func Mat4LookAt(_, _, _ Vec3) Mat4 { return Mat4Identity() }

func Mat4Perspective(_, _, _, _ Scalar) Mat4 { return Mat4Identity() }

func Mat4Ortho(_, _, _, _, _, _ Scalar) Mat4 { return Mat4Identity() }

// Helpers for fixed-point constants.
func ScalarFromFloat32(v float32) Scalar { return Scalar(int32(v * float32(fixedOne))) }

func ScalarToFloat32(v Scalar) float32 { return float32(v) / float32(fixedOne) }

func scalarFromF32(v float32) Scalar { return ScalarFromFloat32(v) }
func scalarToF32(v Scalar) float32   { return ScalarToFloat32(v) }
