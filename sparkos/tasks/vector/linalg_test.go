package vector

import (
	"math"
	"testing"
)

func TestLinAlg_Solve(t *testing.T) {
	e := newEnv()
	_ = e

	// A = [[2,0],[0,4]], b=[2,8] -> x=[1,2].
	A := MatrixValue(2, 2, []float64{2, 0, 0, 4})
	b := ArrayValue([]float64{2, 8})

	v, ok, err := builtinCallLinAlg(nil, "solve", []Value{A, b})
	if !ok || err != nil {
		t.Fatalf("solve ok=%v err=%v", ok, err)
	}
	if v.kind != valueArray || len(v.arr) != 2 {
		t.Fatalf("solve kind=%v", v.kind)
	}
	if math.Abs(v.arr[0]-1) > 1e-12 || math.Abs(v.arr[1]-2) > 1e-12 {
		t.Fatalf("solve=%v", v.arr)
	}
}

func TestLinAlg_QR(t *testing.T) {
	e := newEnv()
	A := MatrixValue(2, 2, []float64{1, 2, 3, 4})

	qv, ok, err := builtinCallLinAlg(e, "qr", []Value{A})
	if !ok || err != nil {
		t.Fatalf("qr ok=%v err=%v", ok, err)
	}
	if qv.kind != valueMatrix || qv.rows != 2 || qv.cols != 2 {
		t.Fatalf("qr Q kind=%v rows=%d cols=%d", qv.kind, qv.rows, qv.cols)
	}
	rv, ok := e.vars["_R"]
	if !ok || rv.kind != valueMatrix || rv.rows != 2 || rv.cols != 2 {
		t.Fatalf("_R=%v kind=%v", rv, rv.kind)
	}

	qt, err := matrixTranspose(qv.rows, qv.cols, qv.mat)
	if err != nil {
		t.Fatalf("transpose: %v", err)
	}
	qtq, err := matrixMul(qv.cols, qv.rows, qt, qv.rows, qv.cols, qv.mat)
	if err != nil {
		t.Fatalf("qtq: %v", err)
	}
	// Q^T Q ~= I.
	if math.Abs(qtq[0]-1) > 1e-6 || math.Abs(qtq[3]-1) > 1e-6 || math.Abs(qtq[1]) > 1e-6 || math.Abs(qtq[2]) > 1e-6 {
		t.Fatalf("qtq=%v", qtq)
	}

	qr, err := matrixMul(qv.rows, qv.cols, qv.mat, rv.rows, rv.cols, rv.mat)
	if err != nil {
		t.Fatalf("qr: %v", err)
	}
	for i := range A.mat {
		if math.Abs(qr[i]-A.mat[i]) > 1e-5 {
			t.Fatalf("QR[%d]=%v want %v", i, qr[i], A.mat[i])
		}
	}
}

func TestLinAlg_SVD(t *testing.T) {
	e := newEnv()
	A := MatrixValue(2, 2, []float64{1, 0, 0, 2})

	sv, ok, err := builtinCallLinAlg(e, "svd", []Value{A})
	if !ok || err != nil {
		t.Fatalf("svd ok=%v err=%v", ok, err)
	}
	if sv.kind != valueArray || len(sv.arr) != 2 {
		t.Fatalf("svd kind=%v len=%d", sv.kind, len(sv.arr))
	}
	if math.Abs(sv.arr[0]-2) > 1e-6 || math.Abs(sv.arr[1]-1) > 1e-6 {
		t.Fatalf("s=%v", sv.arr)
	}
	uv := e.vars["_U"]
	vv := e.vars["_V"]
	if uv.kind != valueMatrix || vv.kind != valueMatrix {
		t.Fatalf("_U kind=%v _V kind=%v", uv.kind, vv.kind)
	}
	// Reconstruct A ~= U*diag(s)*V^T.
	sdiag := MatrixValue(2, 2, []float64{sv.arr[0], 0, 0, sv.arr[1]})
	us, err := matrixMul(uv.rows, uv.cols, uv.mat, sdiag.rows, sdiag.cols, sdiag.mat)
	if err != nil {
		t.Fatalf("U*S: %v", err)
	}
	vt, err := matrixTranspose(vv.rows, vv.cols, vv.mat)
	if err != nil {
		t.Fatalf("V^T: %v", err)
	}
	usvt, err := matrixMul(uv.rows, uv.cols, us, vv.cols, vv.rows, vt)
	if err != nil {
		t.Fatalf("U*S*V^T: %v", err)
	}
	for i := range A.mat {
		if math.Abs(usvt[i]-A.mat[i]) > 1e-4 {
			t.Fatalf("recon[%d]=%v want %v", i, usvt[i], A.mat[i])
		}
	}
}
