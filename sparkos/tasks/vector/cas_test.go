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

func TestCAS_GCDLCMResultantFactor(t *testing.T) {
	e := newEnv()

	// p=x^2-1, q=x^2-x.
	p := nodeBinary{
		op: '-',
		left: nodeBinary{
			op:    '^',
			left:  nodeIdent{name: "x"},
			right: nodeNumber{v: RatNumber(RatInt(2))},
		},
		right: nodeNumber{v: RatNumber(RatInt(1))},
	}
	q := nodeBinary{
		op: '-',
		left: nodeBinary{
			op:    '^',
			left:  nodeIdent{name: "x"},
			right: nodeNumber{v: RatNumber(RatInt(2))},
		},
		right: nodeIdent{name: "x"},
	}

	gv, err := nodeCall{name: "gcd", args: []node{p, q}}.Eval(e)
	if err != nil {
		t.Fatalf("gcd eval: %v", err)
	}
	if gv.kind != valueExpr {
		t.Fatalf("gcd kind=%v", gv.kind)
	}

	lv, err := nodeCall{name: "lcm", args: []node{p, q}}.Eval(e)
	if err != nil {
		t.Fatalf("lcm eval: %v", err)
	}
	if lv.kind != valueExpr {
		t.Fatalf("lcm kind=%v", lv.kind)
	}

	// Check lcm ~= x^3 - x.
	for _, x := range []float64{-3, -1, 0, 2, 5} {
		e.vars["x"] = NumberValue(Float(x))
		got, err := lv.expr.Eval(e)
		if err != nil {
			t.Fatalf("lcm eval x=%v: %v", x, err)
		}
		if !got.IsNumber() {
			t.Fatalf("lcm result kind=%v", got.kind)
		}
		want := x*x*x - x
		if math.Abs(got.num.Float64()-want) > 1e-9 {
			t.Fatalf("lcm x=%v got=%v want=%v", x, got.num.Float64(), want)
		}
	}

	// resultant(x-2, x^2-1, x) == (2^2-1) == 3.
	f1 := nodeBinary{op: '-', left: nodeIdent{name: "x"}, right: nodeNumber{v: RatNumber(RatInt(2))}}
	f2 := p
	rv, err := nodeCall{name: "resultant", args: []node{f1, f2, nodeIdent{name: "x"}}}.Eval(e)
	if err != nil {
		t.Fatalf("resultant eval: %v", err)
	}
	if !rv.IsNumber() || math.Abs(rv.num.Float64()-3) > 1e-9 {
		t.Fatalf("resultant=%v kind=%v", rv.num.Float64(), rv.kind)
	}

	// factor(x^2-1) should match (x-1)*(x+1) up to order.
	fv, err := nodeCall{name: "factor", args: []node{p}}.Eval(e)
	if err != nil {
		t.Fatalf("factor eval: %v", err)
	}
	if fv.kind != valueExpr {
		t.Fatalf("factor kind=%v", fv.kind)
	}
	for _, x := range []float64{-2, -1, 0, 1, 3} {
		e.vars["x"] = NumberValue(Float(x))
		got, err := fv.expr.Eval(e)
		if err != nil {
			t.Fatalf("factor eval x=%v: %v", x, err)
		}
		if !got.IsNumber() {
			t.Fatalf("factor result kind=%v", got.kind)
		}
		want := x*x - 1
		if math.Abs(got.num.Float64()-want) > 1e-9 {
			t.Fatalf("factor x=%v got=%v want=%v", x, got.num.Float64(), want)
		}
	}
}
