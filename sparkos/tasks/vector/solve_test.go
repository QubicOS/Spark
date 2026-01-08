package vector

import (
	"math"
	"testing"
)

func TestSolve1(t *testing.T) {
	e := newEnv()
	f := ExprValue(nodeBinary{op: '-', left: nodeBinary{op: '^', left: nodeIdent{name: "x"}, right: nodeNumber{v: Float(2)}}, right: nodeNumber{v: Float(2)}})

	v, ok, err := builtinCallSolve(e, "solve1", []Value{f, NumberValue(Float(1))})
	if !ok || err != nil {
		t.Fatalf("solve1 ok=%v err=%v", ok, err)
	}
	if !v.IsNumber() {
		t.Fatalf("solve1 kind=%v", v.kind)
	}
	if math.Abs(v.num.Float64()-math.Sqrt2) > 1e-6 {
		t.Fatalf("solve1=%v", v.num.Float64())
	}
}

func TestSolve2(t *testing.T) {
	e := newEnv()

	// x^2 + y^2 = 1, x - y = 0.
	f := ExprValue(nodeBinary{
		op: '-',
		left: nodeBinary{
			op: '+',
			left: nodeBinary{
				op:    '^',
				left:  nodeIdent{name: "x"},
				right: nodeNumber{v: Float(2)},
			},
			right: nodeBinary{
				op:    '^',
				left:  nodeIdent{name: "y"},
				right: nodeNumber{v: Float(2)},
			},
		},
		right: nodeNumber{v: Float(1)},
	})
	g := ExprValue(nodeBinary{op: '-', left: nodeIdent{name: "x"}, right: nodeIdent{name: "y"}})

	v, ok, err := builtinCallSolve(e, "solve2", []Value{f, g, NumberValue(Float(0.8)), NumberValue(Float(0.6))})
	if !ok || err != nil {
		t.Fatalf("solve2 ok=%v err=%v", ok, err)
	}
	if v.kind != valueArray || len(v.arr) != 2 {
		t.Fatalf("solve2 kind=%v", v.kind)
	}
	want := math.Sqrt(0.5)
	if math.Abs(v.arr[0]-want) > 1e-6 || math.Abs(v.arr[1]-want) > 1e-6 {
		t.Fatalf("solve2=%v want=%v", v.arr, want)
	}
}

func TestRoots(t *testing.T) {
	e := newEnv()
	f := ExprValue(nodeCall{name: "sin", args: []node{nodeIdent{name: "x"}}})

	v, ok, err := builtinCallSolve(e, "roots", []Value{f, NumberValue(Float(0)), NumberValue(Float(math.Pi * 2)), NumberValue(Float(256))})
	if !ok || err != nil {
		t.Fatalf("roots ok=%v err=%v", ok, err)
	}
	if v.kind != valueArray || len(v.arr) == 0 {
		t.Fatalf("roots kind=%v len=%d", v.kind, len(v.arr))
	}
	foundPi := false
	for _, r := range v.arr {
		if math.Abs(r-math.Pi) < 1e-3 {
			foundPi = true
		}
	}
	if !foundPi {
		t.Fatalf("roots=%v (expected ~pi)", v.arr)
	}
}

func TestRegion(t *testing.T) {
	e := newEnv()
	// x^2 + y^2 <= 1.
	cond := ExprValue(nodeCompare{
		op: tokLe,
		left: nodeBinary{
			op: '+',
			left: nodeBinary{
				op:    '^',
				left:  nodeIdent{name: "x"},
				right: nodeNumber{v: Float(2)},
			},
			right: nodeBinary{
				op:    '^',
				left:  nodeIdent{name: "y"},
				right: nodeNumber{v: Float(2)},
			},
		},
		right: nodeNumber{v: Float(1)},
	})

	v, ok, err := builtinCallSolve(e, "region", []Value{
		cond,
		NumberValue(Float(-1)), NumberValue(Float(1)),
		NumberValue(Float(-1)), NumberValue(Float(1)),
		NumberValue(Float(16)),
	})
	if !ok || err != nil {
		t.Fatalf("region ok=%v err=%v", ok, err)
	}
	if v.kind != valueMatrix || v.cols != 2 || v.rows <= 0 {
		t.Fatalf("region kind=%v rows=%d cols=%d", v.kind, v.rows, v.cols)
	}
}
