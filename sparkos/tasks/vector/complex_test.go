package vector

import (
	"math"
	"testing"
)

func TestComplexArithmeticAndBuiltins(t *testing.T) {
	e := newEnv()
	e.vars["z"] = ComplexValue(3, 4)

	absV, err := nodeCall{name: "abs", args: []node{nodeIdent{name: "z"}}}.Eval(e)
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	if !absV.IsNumber() || math.Abs(absV.num.Float64()-5) > 1e-12 {
		t.Fatalf("abs=%v", absV.num.Float64())
	}

	reV, err := nodeCall{name: "re", args: []node{nodeIdent{name: "z"}}}.Eval(e)
	if err != nil {
		t.Fatalf("re: %v", err)
	}
	if !reV.IsNumber() || reV.num.Float64() != 3 {
		t.Fatalf("re=%v", reV.num.Float64())
	}

	imV, err := nodeCall{name: "im", args: []node{nodeIdent{name: "z"}}}.Eval(e)
	if err != nil {
		t.Fatalf("im: %v", err)
	}
	if !imV.IsNumber() || imV.num.Float64() != 4 {
		t.Fatalf("im=%v", imV.num.Float64())
	}

	// (1+2i) + 3 = 4+2i.
	addV, err := nodeBinary{
		op:    '+',
		left:  nodeBinary{op: '+', left: nodeNumber{v: Float(1)}, right: nodeBinary{op: '*', left: nodeNumber{v: Float(2)}, right: nodeIdent{name: "i"}}},
		right: nodeNumber{v: Float(3)},
	}.Eval(e)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if addV.kind != valueComplex || real(addV.c) != 4 || imag(addV.c) != 2 {
		t.Fatalf("add=%v", addV.c)
	}

	// i^2 = -1.
	powV, err := nodeBinary{op: '^', left: nodeIdent{name: "i"}, right: nodeNumber{v: Float(2)}}.Eval(e)
	if err != nil {
		t.Fatalf("pow: %v", err)
	}
	if powV.kind != valueComplex || math.Abs(real(powV.c)+1) > 1e-12 || math.Abs(imag(powV.c)) > 1e-12 {
		t.Fatalf("i^2=%v", powV.c)
	}
}
