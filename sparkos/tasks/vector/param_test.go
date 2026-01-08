package vector

import (
	"math"
	"testing"
)

func TestParametricPlotBuiltin(t *testing.T) {
	e := newEnv()

	v, err := nodeCall{name: "param", args: []node{
		nodeCall{name: "cos", args: []node{nodeIdent{name: "t"}}},
		nodeCall{name: "sin", args: []node{nodeIdent{name: "t"}}},
		nodeNumber{v: Float(0)},
		nodeBinary{op: '/', left: nodeIdent{name: "pi"}, right: nodeNumber{v: Float(2)}},
		nodeNumber{v: Float(3)},
	}}.Eval(e)
	if err != nil {
		t.Fatalf("param: %v", err)
	}
	if v.kind != valueMatrix || v.rows != 3 || v.cols != 2 {
		t.Fatalf("param kind=%v rows=%d cols=%d", v.kind, v.rows, v.cols)
	}

	// t = 0: (1,0)
	if math.Abs(v.mat[0]-1) > 1e-12 || math.Abs(v.mat[1]-0) > 1e-12 {
		t.Fatalf("p0=%v,%v", v.mat[0], v.mat[1])
	}
	// t = pi/4: (sqrt(2)/2, sqrt(2)/2)
	s2 := math.Sqrt2 / 2
	if math.Abs(v.mat[2]-s2) > 1e-12 || math.Abs(v.mat[3]-s2) > 1e-12 {
		t.Fatalf("p1=%v,%v", v.mat[2], v.mat[3])
	}
	// t = pi/2: (0,1)
	if math.Abs(v.mat[4]-0) > 1e-12 || math.Abs(v.mat[5]-1) > 1e-12 {
		t.Fatalf("p2=%v,%v", v.mat[4], v.mat[5])
	}
}
