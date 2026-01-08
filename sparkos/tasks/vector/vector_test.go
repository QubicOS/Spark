package vector

import (
	"math"
	"testing"
)

func TestVectorOps(t *testing.T) {
	e := newEnv()
	e.vars["a"] = ArrayValue([]float64{1, 0, 0})
	e.vars["b"] = ArrayValue([]float64{0, 1, 0})

	dv, err := nodeCall{name: "dot", args: []node{nodeIdent{name: "a"}, nodeIdent{name: "b"}}}.Eval(e)
	if err != nil {
		t.Fatalf("dot: %v", err)
	}
	if !dv.IsNumber() || dv.num.Float64() != 0 {
		t.Fatalf("dot=%v kind=%v", dv.num.Float64(), dv.kind)
	}

	cv, err := nodeCall{name: "cross", args: []node{nodeIdent{name: "a"}, nodeIdent{name: "b"}}}.Eval(e)
	if err != nil {
		t.Fatalf("cross: %v", err)
	}
	if cv.kind != valueArray || len(cv.arr) != 3 || cv.arr[0] != 0 || cv.arr[1] != 0 || cv.arr[2] != 1 {
		t.Fatalf("cross=%v kind=%v", cv.arr, cv.kind)
	}

	nv, err := nodeCall{name: "norm", args: []node{nodeCall{name: "vec3", args: []node{
		nodeNumber{v: Float(3)},
		nodeNumber{v: Float(4)},
		nodeNumber{v: Float(0)},
	}}}}.Eval(e)
	if err != nil {
		t.Fatalf("norm: %v", err)
	}
	if !nv.IsNumber() || nv.num.Float64() != 5 {
		t.Fatalf("norm=%v kind=%v", nv.num.Float64(), nv.kind)
	}

	uv, err := nodeCall{name: "unit", args: []node{nodeCall{name: "vec2", args: []node{
		nodeNumber{v: Float(3)},
		nodeNumber{v: Float(4)},
	}}}}.Eval(e)
	if err != nil {
		t.Fatalf("unit: %v", err)
	}
	if uv.kind != valueArray || len(uv.arr) != 2 || math.Abs(uv.arr[0]-0.6) > 1e-12 || math.Abs(uv.arr[1]-0.8) > 1e-12 {
		t.Fatalf("unit=%v kind=%v", uv.arr, uv.kind)
	}

	av, err := nodeCall{name: "angle", args: []node{nodeIdent{name: "a"}, nodeIdent{name: "b"}}}.Eval(e)
	if err != nil {
		t.Fatalf("angle: %v", err)
	}
	if !av.IsNumber() || math.Abs(av.num.Float64()-math.Pi/2) > 1e-12 {
		t.Fatalf("angle=%v", av.num.Float64())
	}

	dstv, err := nodeCall{name: "dist", args: []node{
		nodeCall{name: "vec2", args: []node{nodeNumber{v: Float(0)}, nodeNumber{v: Float(0)}}},
		nodeCall{name: "vec2", args: []node{nodeNumber{v: Float(3)}, nodeNumber{v: Float(4)}}},
	}}.Eval(e)
	if err != nil {
		t.Fatalf("dist: %v", err)
	}
	if !dstv.IsNumber() || dstv.num.Float64() != 5 {
		t.Fatalf("dist=%v", dstv.num.Float64())
	}

	ov, err := nodeCall{name: "outer", args: []node{
		nodeCall{name: "vec2", args: []node{nodeNumber{v: Float(1)}, nodeNumber{v: Float(2)}}},
		nodeCall{name: "vec3", args: []node{nodeNumber{v: Float(3)}, nodeNumber{v: Float(4)}, nodeNumber{v: Float(5)}}},
	}}.Eval(e)
	if err != nil {
		t.Fatalf("outer: %v", err)
	}
	if ov.kind != valueMatrix || ov.rows != 2 || ov.cols != 3 {
		t.Fatalf("outer kind=%v rows=%d cols=%d", ov.kind, ov.rows, ov.cols)
	}
	if ov.mat[0] != 3 || ov.mat[1] != 4 || ov.mat[2] != 5 || ov.mat[3] != 6 || ov.mat[4] != 8 || ov.mat[5] != 10 {
		t.Fatalf("outer mat=%v", ov.mat)
	}
}

func TestVectorGetSetComponents(t *testing.T) {
	e := newEnv()
	e.vars["v"] = ArrayValue([]float64{10, 20, 30, 40})

	gv, err := nodeCall{name: "get", args: []node{nodeIdent{name: "v"}, nodeNumber{v: Float(3)}}}.Eval(e)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !gv.IsNumber() || gv.num.Float64() != 30 {
		t.Fatalf("get=%v kind=%v", gv.num.Float64(), gv.kind)
	}

	sv, err := nodeCall{name: "set", args: []node{nodeIdent{name: "v"}, nodeNumber{v: Float(2)}, nodeNumber{v: Float(99)}}}.Eval(e)
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if sv.kind != valueArray || len(sv.arr) != 4 || sv.arr[1] != 99 {
		t.Fatalf("set=%v kind=%v", sv.arr, sv.kind)
	}

	xv, err := nodeCall{name: "x", args: []node{nodeIdent{name: "v"}}}.Eval(e)
	if err != nil {
		t.Fatalf("x: %v", err)
	}
	if !xv.IsNumber() || xv.num.Float64() != 10 {
		t.Fatalf("x=%v", xv.num.Float64())
	}

	zv, err := nodeCall{name: "z", args: []node{nodeIdent{name: "v"}}}.Eval(e)
	if err != nil {
		t.Fatalf("z: %v", err)
	}
	if !zv.IsNumber() || zv.num.Float64() != 30 {
		t.Fatalf("z=%v", zv.num.Float64())
	}
}
