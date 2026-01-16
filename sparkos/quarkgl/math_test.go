//go:build !quarkgl_fixed

package quarkgl

import "testing"

func TestMat4MulIdentity(t *testing.T) {
	a := Mat4Identity()
	b := Mat4Translate(V3(1, 2, 3))
	got := Mat4Mul(a, b)
	if got != b {
		t.Fatalf("identity*a mismatch")
	}
	got2 := Mat4Mul(b, a)
	if got2 != b {
		t.Fatalf("a*identity mismatch")
	}
}

func TestLookAtNotIdentity(t *testing.T) {
	m := Mat4LookAt(V3(0, 0, 3), V3(0, 0, 0), V3(0, 1, 0))
	if m == Mat4Identity() {
		t.Fatalf("lookAt unexpectedly identity")
	}
}
