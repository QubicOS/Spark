package vector

import (
	"math"
	"testing"
)

func TestPoly_PolyvalPolyfitRoots(t *testing.T) {
	e := newEnv()

	// p(x) = x^2 - 1 => coeffs [-1, 0, 1].
	coeffs := ArrayValue([]float64{-1, 0, 1})

	v, ok, err := builtinCallPoly(e, "polyval", []Value{coeffs, NumberValue(Float(3))})
	if !ok || err != nil {
		t.Fatalf("polyval ok=%v err=%v", ok, err)
	}
	if !v.IsNumber() || v.num.Float64() != 8 {
		t.Fatalf("polyval=%v", v.num.Float64())
	}

	// Roots should be near -1 and +1 (returned as Nx2 matrix of complex).
	rv, ok, err := builtinCallPoly(e, "roots", []Value{coeffs})
	if !ok || err != nil {
		t.Fatalf("roots ok=%v err=%v", ok, err)
	}
	if rv.kind != valueMatrix || rv.cols != 2 || rv.rows != 2 {
		t.Fatalf("roots kind=%v rows=%d cols=%d", rv.kind, rv.rows, rv.cols)
	}
	// Extract real parts and check we have +/-1.
	r0 := rv.mat[0]
	r1 := rv.mat[2]
	if math.Abs(math.Abs(r0)-1) > 1e-6 || math.Abs(math.Abs(r1)-1) > 1e-6 {
		t.Fatalf("roots=%v", []float64{r0, rv.mat[1], r1, rv.mat[3]})
	}

	// Fit y = 2 + 3x from 3 points.
	xs := ArrayValue([]float64{0, 1, 2})
	ys := ArrayValue([]float64{2, 5, 8})
	fv, ok, err := builtinCallPoly(e, "polyfit", []Value{xs, ys, NumberValue(Float(1))})
	if !ok || err != nil {
		t.Fatalf("polyfit ok=%v err=%v", ok, err)
	}
	if fv.kind != valueArray || len(fv.arr) != 2 {
		t.Fatalf("polyfit kind=%v", fv.kind)
	}
	if math.Abs(fv.arr[0]-2) > 1e-9 || math.Abs(fv.arr[1]-3) > 1e-9 {
		t.Fatalf("polyfit coeffs=%v", fv.arr)
	}
}
