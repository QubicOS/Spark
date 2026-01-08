package vector

import (
	"math"
	"testing"
)

func TestPlaneBuiltin(t *testing.T) {
	e := newEnv()

	// Plane z = x + 2y + 3 => x + 2y - z + 3 = 0 => n=(1,2,-1), d=3.
	v, ok, err := builtinCallPlane(e, "plane", []Value{
		ArrayValue([]float64{1, 2, -1}),
		NumberValue(Float(3)),
	})
	if !ok || err != nil {
		t.Fatalf("plane ok=%v err=%v", ok, err)
	}
	if v.kind != valueExpr {
		t.Fatalf("plane kind=%v", v.kind)
	}

	e.vars["x"] = NumberValue(Float(10))
	e.vars["y"] = NumberValue(Float(5))
	out, err := v.expr.Eval(e)
	if err != nil {
		t.Fatalf("plane eval: %v", err)
	}
	if !out.IsNumber() {
		t.Fatalf("plane out kind=%v", out.kind)
	}
	// 10 + 2*5 + 3 = 23.
	if math.Abs(out.num.Float64()-23) > 1e-12 {
		t.Fatalf("plane=%v", out.num.Float64())
	}

	// Through points: (0,0,0), (1,0,1), (0,1,2) => z = x + 2y.
	v2, ok, err := builtinCallPlane(e, "plane", []Value{
		ArrayValue([]float64{0, 0, 0}),
		ArrayValue([]float64{1, 0, 1}),
		ArrayValue([]float64{0, 1, 2}),
	})
	if !ok || err != nil {
		t.Fatalf("plane(p0,p1,p2) ok=%v err=%v", ok, err)
	}
	if v2.kind != valueExpr {
		t.Fatalf("plane2 kind=%v", v2.kind)
	}
	e.vars["x"] = NumberValue(Float(3))
	e.vars["y"] = NumberValue(Float(4))
	out2, err := v2.expr.Eval(e)
	if err != nil {
		t.Fatalf("plane2 eval: %v", err)
	}
	if !out2.IsNumber() || math.Abs(out2.num.Float64()-(3+2*4)) > 1e-12 {
		t.Fatalf("plane2=%v", out2.num.Float64())
	}
}
