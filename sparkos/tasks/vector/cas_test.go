package vector

import (
	"math"
	"testing"
)

func TestCAS_SeriesSin(t *testing.T) {
	e := newEnv()

	v, err := nodeCall{
		name: "series",
		args: []node{
			nodeCall{name: "sin", args: []node{nodeIdent{name: "x"}}},
			nodeIdent{name: "x"},
			nodeNumber{v: Float(0)},
			nodeNumber{v: Float(5)},
		},
	}.Eval(e)
	if err != nil {
		t.Fatalf("series eval: %v", err)
	}
	if v.kind != valueExpr {
		t.Fatalf("series kind=%v", v.kind)
	}

	x := 0.2
	e.vars["x"] = NumberValue(Float(x))
	gotV, err := v.expr.Eval(e)
	if err != nil {
		t.Fatalf("series expr eval: %v", err)
	}
	if !gotV.IsNumber() {
		t.Fatalf("series expr kind=%v", gotV.kind)
	}
	got := gotV.num.Float64()
	want := x - (x*x*x)/6 + (x*x*x*x*x)/120
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("series=%v want=%v", got, want)
	}
}

func TestCAS_Expand(t *testing.T) {
	e := newEnv()

	v, err := nodeCall{
		name: "expand",
		args: []node{
			nodeBinary{
				op: '^',
				left: nodeBinary{
					op:    '+',
					left:  nodeIdent{name: "x"},
					right: nodeNumber{v: Float(1)},
				},
				right: nodeNumber{v: Float(3)},
			},
		},
	}.Eval(e)
	if err != nil {
		t.Fatalf("expand eval: %v", err)
	}
	if v.kind != valueExpr {
		t.Fatalf("expand kind=%v", v.kind)
	}

	for _, x := range []float64{-2, -0.5, 0, 0.25, 2} {
		e.vars["x"] = NumberValue(Float(x))
		gotV, err := v.expr.Eval(e)
		if err != nil {
			t.Fatalf("expand expr eval x=%v: %v", x, err)
		}
		if !gotV.IsNumber() {
			t.Fatalf("expand expr kind=%v", gotV.kind)
		}
		got := gotV.num.Float64()
		want := math.Pow(x+1, 3)
		if math.Abs(got-want) > 1e-9 {
			t.Fatalf("expand x=%v got=%v want=%v", x, got, want)
		}
	}
}

func TestCAS_PolynomialIntrospection(t *testing.T) {
	e := newEnv()

	// p(x) = x^3 + 2*x + 1.
	p := nodeBinary{
		op: '+',
		left: nodeBinary{
			op: '+',
			left: nodeBinary{
				op:    '^',
				left:  nodeIdent{name: "x"},
				right: nodeNumber{v: Float(3)},
			},
			right: nodeBinary{
				op: '*',
				left: nodeNumber{
					v: Float(2),
				},
				right: nodeIdent{name: "x"},
			},
		},
		right: nodeNumber{v: Float(1)},
	}

	dv, err := nodeCall{name: "degree", args: []node{p, nodeIdent{name: "x"}}}.Eval(e)
	if err != nil {
		t.Fatalf("degree eval: %v", err)
	}
	if !dv.IsNumber() || dv.num.Float64() != 3 {
		t.Fatalf("degree=%v kind=%v", dv.num.Float64(), dv.kind)
	}

	cv, err := nodeCall{name: "coeff", args: []node{p, nodeIdent{name: "x"}, nodeNumber{v: Float(1)}}}.Eval(e)
	if err != nil {
		t.Fatalf("coeff eval: %v", err)
	}
	if !cv.IsNumber() || cv.num.Float64() != 2 {
		t.Fatalf("coeff=%v kind=%v", cv.num.Float64(), cv.kind)
	}

	hv, err := nodeCall{name: "collect", args: []node{p, nodeIdent{name: "x"}}}.Eval(e)
	if err != nil {
		t.Fatalf("collect eval: %v", err)
	}
	if hv.kind != valueExpr {
		t.Fatalf("collect kind=%v", hv.kind)
	}

	for _, x := range []float64{-2, -0.5, 0, 1, 3.5} {
		e.vars["x"] = NumberValue(Float(x))
		gotV, err := hv.expr.Eval(e)
		if err != nil {
			t.Fatalf("collect expr eval x=%v: %v", x, err)
		}
		if !gotV.IsNumber() {
			t.Fatalf("collect expr kind=%v", gotV.kind)
		}
		got := gotV.num.Float64()
		want := x*x*x + 2*x + 1
		if math.Abs(got-want) > 1e-9 {
			t.Fatalf("collect x=%v got=%v want=%v", x, got, want)
		}
	}
}
