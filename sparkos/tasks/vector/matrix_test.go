package vector

import (
	"math"
	"testing"
)

func TestMatrixReshapeTransposeShape(t *testing.T) {
	e := newEnv()

	v, err := nodeCall{
		name: "reshape",
		args: []node{
			nodeCall{name: "range", args: []node{nodeNumber{v: Float(1)}, nodeNumber{v: Float(6)}, nodeNumber{v: Float(6)}}},
			nodeNumber{v: Float(3)},
			nodeNumber{v: Float(2)},
		},
	}.Eval(e)
	if err != nil {
		t.Fatalf("reshape eval: %v", err)
	}
	if v.kind != valueMatrix || v.rows != 3 || v.cols != 2 {
		t.Fatalf("reshape kind=%v rows=%d cols=%d", v.kind, v.rows, v.cols)
	}
	want := []float64{1, 2, 3, 4, 5, 6}
	for i := range want {
		if v.mat[i] != want[i] {
			t.Fatalf("reshape[%d]=%v, want %v", i, v.mat[i], want[i])
		}
	}

	tv, err := nodeCall{name: "T", args: []node{nodeCall{name: "reshape", args: []node{
		nodeCall{name: "range", args: []node{nodeNumber{v: Float(1)}, nodeNumber{v: Float(6)}, nodeNumber{v: Float(6)}}},
		nodeNumber{v: Float(3)},
		nodeNumber{v: Float(2)},
	}}}}.Eval(e)
	if err != nil {
		t.Fatalf("T eval: %v", err)
	}
	if tv.kind != valueMatrix || tv.rows != 2 || tv.cols != 3 {
		t.Fatalf("T kind=%v rows=%d cols=%d", tv.kind, tv.rows, tv.cols)
	}
	if tv.mat[0] != 1 || tv.mat[1] != 3 || tv.mat[2] != 5 || tv.mat[3] != 2 || tv.mat[4] != 4 || tv.mat[5] != 6 {
		t.Fatalf("T data=%v", tv.mat)
	}

	e.vars["M"] = v
	sv, err := nodeCall{name: "shape", args: []node{nodeIdent{name: "M"}}}.Eval(e)
	if err != nil {
		t.Fatalf("shape eval: %v", err)
	}
	if sv.kind != valueArray || len(sv.arr) != 2 || sv.arr[0] != 3 || sv.arr[1] != 2 {
		t.Fatalf("shape=%v kind=%v", sv.arr, sv.kind)
	}
}

func TestMatrixMulDetInv(t *testing.T) {
	e := newEnv()

	// Matmul shape check: (2x3) * (3x2) -> (2x2).
	e.vars["A"] = MatrixValue(2, 3, []float64{1, 2, 3, 4, 5, 6})
	e.vars["B"] = MatrixValue(3, 2, []float64{7, 8, 9, 10, 11, 12})
	mv, err := nodeBinary{op: '*', left: nodeIdent{name: "A"}, right: nodeIdent{name: "B"}}.Eval(e)
	if err != nil {
		t.Fatalf("matmul: %v", err)
	}
	if mv.kind != valueMatrix || mv.rows != 2 || mv.cols != 2 {
		t.Fatalf("matmul kind=%v rows=%d cols=%d", mv.kind, mv.rows, mv.cols)
	}
	// [[58,64],[139,154]]
	if mv.mat[0] != 58 || mv.mat[1] != 64 || mv.mat[2] != 139 || mv.mat[3] != 154 {
		t.Fatalf("matmul data=%v", mv.mat)
	}

	// A = [[4,7],[2,6]].
	e.vars["A"] = MatrixValue(2, 2, []float64{4, 7, 2, 6})

	dv, err := nodeCall{name: "det", args: []node{nodeIdent{name: "A"}}}.Eval(e)
	if err != nil {
		t.Fatalf("det: %v", err)
	}
	if !dv.IsNumber() {
		t.Fatalf("det kind=%v", dv.kind)
	}
	if math.Abs(dv.num.Float64()-10) > 1e-9 {
		t.Fatalf("det=%v", dv.num.Float64())
	}

	iv, err := nodeCall{name: "inv", args: []node{nodeIdent{name: "A"}}}.Eval(e)
	if err != nil {
		t.Fatalf("inv: %v", err)
	}
	if iv.kind != valueMatrix || iv.rows != 2 || iv.cols != 2 {
		t.Fatalf("inv kind=%v rows=%d cols=%d", iv.kind, iv.rows, iv.cols)
	}

	// A * inv(A) ~= I.
	mv, err = nodeBinary{op: '*', left: nodeIdent{name: "A"}, right: nodeCall{name: "inv", args: []node{nodeIdent{name: "A"}}}}.Eval(e)
	if err != nil {
		t.Fatalf("mul: %v", err)
	}
	if mv.kind != valueMatrix || mv.rows != 2 || mv.cols != 2 {
		t.Fatalf("mul kind=%v", mv.kind)
	}
	want := []float64{1, 0, 0, 1}
	for i := range want {
		if math.Abs(mv.mat[i]-want[i]) > 1e-6 {
			t.Fatalf("I[%d]=%v", i, mv.mat[i])
		}
	}
}

func TestMatrixIndexingAndStats(t *testing.T) {
	e := newEnv()
	e.vars["A"] = MatrixValue(2, 3, []float64{1, 2, 3, 4, 5, 6})

	gv, err := nodeCall{name: "get", args: []node{nodeIdent{name: "A"}, nodeNumber{v: Float(2)}, nodeNumber{v: Float(3)}}}.Eval(e)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !gv.IsNumber() || gv.num.Float64() != 6 {
		t.Fatalf("get=%v kind=%v", gv.num.Float64(), gv.kind)
	}

	sv, err := nodeCall{name: "set", args: []node{nodeIdent{name: "A"}, nodeNumber{v: Float(1)}, nodeNumber{v: Float(2)}, nodeNumber{v: Float(99)}}}.Eval(e)
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if sv.kind != valueMatrix || sv.rows != 2 || sv.cols != 3 || sv.mat[1] != 99 {
		t.Fatalf("set kind=%v rows=%d cols=%d mat=%v", sv.kind, sv.rows, sv.cols, sv.mat)
	}

	rv, err := nodeCall{name: "row", args: []node{nodeIdent{name: "A"}, nodeNumber{v: Float(2)}}}.Eval(e)
	if err != nil {
		t.Fatalf("row: %v", err)
	}
	if rv.kind != valueArray || len(rv.arr) != 3 || rv.arr[0] != 4 || rv.arr[1] != 5 || rv.arr[2] != 6 {
		t.Fatalf("row=%v kind=%v", rv.arr, rv.kind)
	}

	cv, err := nodeCall{name: "col", args: []node{nodeIdent{name: "A"}, nodeNumber{v: Float(1)}}}.Eval(e)
	if err != nil {
		t.Fatalf("col: %v", err)
	}
	if cv.kind != valueArray || len(cv.arr) != 2 || cv.arr[0] != 1 || cv.arr[1] != 4 {
		t.Fatalf("col=%v kind=%v", cv.arr, cv.kind)
	}

	e.vars["S"] = MatrixValue(2, 2, []float64{3, 1, 2, 4})
	dv, err := nodeCall{name: "diag", args: []node{nodeIdent{name: "S"}}}.Eval(e)
	if err != nil {
		t.Fatalf("diag: %v", err)
	}
	if dv.kind != valueArray || len(dv.arr) != 2 || dv.arr[0] != 3 || dv.arr[1] != 4 {
		t.Fatalf("diag=%v kind=%v", dv.arr, dv.kind)
	}

	tv, err := nodeCall{name: "trace", args: []node{nodeIdent{name: "S"}}}.Eval(e)
	if err != nil {
		t.Fatalf("trace: %v", err)
	}
	if !tv.IsNumber() || tv.num.Float64() != 7 {
		t.Fatalf("trace=%v kind=%v", tv.num.Float64(), tv.kind)
	}

	nv, err := nodeCall{name: "norm", args: []node{nodeIdent{name: "S"}}}.Eval(e)
	if err != nil {
		t.Fatalf("norm: %v", err)
	}
	// sqrt(3^2+1^2+2^2+4^2) = sqrt(30).
	if !nv.IsNumber() || math.Abs(nv.num.Float64()-math.Sqrt(30)) > 1e-9 {
		t.Fatalf("norm=%v kind=%v", nv.num.Float64(), nv.kind)
	}

	ev, err := nodeCall{name: "sin", args: []node{nodeIdent{name: "S"}}}.Eval(e)
	if err != nil {
		t.Fatalf("sin: %v", err)
	}
	if ev.kind != valueMatrix || ev.rows != 2 || ev.cols != 2 {
		t.Fatalf("sin kind=%v rows=%d cols=%d", ev.kind, ev.rows, ev.cols)
	}
	if math.Abs(ev.mat[0]-math.Sin(3)) > 1e-12 {
		t.Fatalf("sin[0]=%v", ev.mat[0])
	}
}
