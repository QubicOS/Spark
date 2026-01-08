package vector

import "testing"

func TestComparisonsAndConditions(t *testing.T) {
	e := newEnv()

	v, err := nodeCompare{op: tokLt, left: nodeNumber{v: Float(1)}, right: nodeNumber{v: Float(2)}}.Eval(e)
	if err != nil {
		t.Fatalf("1<2: %v", err)
	}
	if !v.IsNumber() || v.num.Float64() != 1 {
		t.Fatalf("1<2=%v kind=%v", v.num.Float64(), v.kind)
	}

	v, err = nodeCall{name: "if", args: []node{
		nodeCompare{op: tokGt, left: nodeNumber{v: Float(1)}, right: nodeNumber{v: Float(2)}},
		nodeNumber{v: Float(10)},
		nodeNumber{v: Float(20)},
	}}.Eval(e)
	if err != nil {
		t.Fatalf("if: %v", err)
	}
	if !v.IsNumber() || v.num.Float64() != 20 {
		t.Fatalf("if=%v kind=%v", v.num.Float64(), v.kind)
	}

	// Named symbolic expression.
	ev, err := nodeCall{name: "expr", args: []node{
		nodeBinary{op: '+', left: nodeIdent{name: "x"}, right: nodeNumber{v: Float(1)}},
	}}.Eval(e)
	if err != nil {
		t.Fatalf("expr: %v", err)
	}
	if ev.kind != valueExpr {
		t.Fatalf("expr kind=%v", ev.kind)
	}
	e.vars["E"] = ev

	// Evaluate it after binding x.
	e.vars["x"] = NumberValue(Float(41))
	out, err := nodeCall{name: "eval", args: []node{nodeIdent{name: "E"}}}.Eval(e)
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	if !out.IsNumber() || out.num.Float64() != 42 {
		t.Fatalf("eval=%v kind=%v", out.num.Float64(), out.kind)
	}
}
