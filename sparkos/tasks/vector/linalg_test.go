package vector

import (
	"math"
	"testing"
)

func TestLinAlg_Solve(t *testing.T) {
	e := newEnv()
	_ = e

	// A = [[2,0],[0,4]], b=[2,8] -> x=[1,2].
	A := MatrixValue(2, 2, []float64{2, 0, 0, 4})
	b := ArrayValue([]float64{2, 8})

	v, ok, err := builtinCallLinAlg(nil, "solve", []Value{A, b})
	if !ok || err != nil {
		t.Fatalf("solve ok=%v err=%v", ok, err)
	}
	if v.kind != valueArray || len(v.arr) != 2 {
		t.Fatalf("solve kind=%v", v.kind)
	}
	if math.Abs(v.arr[0]-1) > 1e-12 || math.Abs(v.arr[1]-2) > 1e-12 {
		t.Fatalf("solve=%v", v.arr)
	}
}
