package vector

import (
	"math"
	"testing"
)

func TestNumeric_NewtonAndBisectionAndSecant(t *testing.T) {
	e := newEnv()

	// f(x)=x^2-2.
	f := ExprValue(nodeBinary{
		op: '-',
		left: nodeBinary{
			op:    '^',
			left:  nodeIdent{name: "x"},
			right: nodeNumber{v: Float(2)},
		},
		right: nodeNumber{v: Float(2)},
	})

	v, ok, err := builtinCallNumeric(e, "newton", []Value{f, NumberValue(Float(1))})
	if !ok || err != nil {
		t.Fatalf("newton ok=%v err=%v", ok, err)
	}
	if !v.IsNumber() || math.Abs(v.num.Float64()-math.Sqrt2) > 1e-6 {
		t.Fatalf("newton=%v", v.num.Float64())
	}

	v, ok, err = builtinCallNumeric(e, "bisection", []Value{f, NumberValue(Float(1)), NumberValue(Float(2))})
	if !ok || err != nil {
		t.Fatalf("bisection ok=%v err=%v", ok, err)
	}
	if !v.IsNumber() || math.Abs(v.num.Float64()-math.Sqrt2) > 1e-6 {
		t.Fatalf("bisection=%v", v.num.Float64())
	}

	v, ok, err = builtinCallNumeric(e, "secant", []Value{f, NumberValue(Float(1)), NumberValue(Float(2))})
	if !ok || err != nil {
		t.Fatalf("secant ok=%v err=%v", ok, err)
	}
	if !v.IsNumber() || math.Abs(v.num.Float64()-math.Sqrt2) > 1e-6 {
		t.Fatalf("secant=%v", v.num.Float64())
	}
}

func TestNumeric_DiffAndIntegrate(t *testing.T) {
	e := newEnv()

	// f(x)=sin(x).
	f := ExprValue(nodeCall{name: "sin", args: []node{nodeIdent{name: "x"}}})

	v, ok, err := builtinCallNumeric(e, "diff_num", []Value{f, NumberValue(Float(0.3))})
	if !ok || err != nil {
		t.Fatalf("diff_num ok=%v err=%v", ok, err)
	}
	if !v.IsNumber() || math.Abs(v.num.Float64()-math.Cos(0.3)) > 1e-4 {
		t.Fatalf("diff_num=%v", v.num.Float64())
	}

	// ∫_0^pi sin(x) dx = 2.
	v, ok, err = builtinCallNumeric(e, "integrate_num", []Value{f, NumberValue(Float(0)), NumberValue(Float(math.Pi)), NumberValue(Float(1)), NumberValue(Float(256))})
	if !ok || err != nil {
		t.Fatalf("integrate_num ok=%v err=%v", ok, err)
	}
	if !v.IsNumber() || math.Abs(v.num.Float64()-2) > 1e-3 {
		t.Fatalf("integrate_num=%v", v.num.Float64())
	}
}
