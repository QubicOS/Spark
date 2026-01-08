package vector

import (
	"math"
	"testing"
)

func TestStats_MedianVarianceStd(t *testing.T) {
	e := newEnv()
	e.vars["x"] = ArrayValue([]float64{1, 3, 2, 4})

	mv, err := nodeCall{name: "median", args: []node{nodeIdent{name: "x"}}}.Eval(e)
	if err != nil {
		t.Fatalf("median: %v", err)
	}
	if !mv.IsNumber() || mv.num.Float64() != 2.5 {
		t.Fatalf("median=%v", mv.num.Float64())
	}

	vv, err := nodeCall{name: "variance", args: []node{nodeIdent{name: "x"}}}.Eval(e)
	if err != nil {
		t.Fatalf("variance: %v", err)
	}
	if !vv.IsNumber() || math.Abs(vv.num.Float64()-1.25) > 1e-12 {
		t.Fatalf("variance=%v", vv.num.Float64())
	}

	sv, err := nodeCall{name: "std", args: []node{nodeIdent{name: "x"}}}.Eval(e)
	if err != nil {
		t.Fatalf("std: %v", err)
	}
	if !sv.IsNumber() || math.Abs(sv.num.Float64()-math.Sqrt(1.25)) > 1e-12 {
		t.Fatalf("std=%v", sv.num.Float64())
	}
}

func TestStats_CovCorrHistConvolve(t *testing.T) {
	e := newEnv()

	x := ArrayValue([]float64{1, 2, 3, 4})
	y := ArrayValue([]float64{2, 4, 6, 8})

	cv, ok, err := builtinCallStats(e, "cov", []Value{x, y})
	if !ok || err != nil {
		t.Fatalf("cov ok=%v err=%v", ok, err)
	}
	if !cv.IsNumber() || math.Abs(cv.num.Float64()-2.5) > 1e-12 {
		t.Fatalf("cov=%v", cv.num.Float64())
	}

	rv, ok, err := builtinCallStats(e, "corr", []Value{x, y})
	if !ok || err != nil {
		t.Fatalf("corr ok=%v err=%v", ok, err)
	}
	if !rv.IsNumber() || math.Abs(rv.num.Float64()-1) > 1e-12 {
		t.Fatalf("corr=%v", rv.num.Float64())
	}

	hv, ok, err := builtinCallStats(e, "hist", []Value{ArrayValue([]float64{0, 1, 2, 3}), NumberValue(Float(2))})
	if !ok || err != nil {
		t.Fatalf("hist ok=%v err=%v", ok, err)
	}
	if hv.kind != valueMatrix || hv.rows != 2 || hv.cols != 2 {
		t.Fatalf("hist kind=%v rows=%d cols=%d", hv.kind, hv.rows, hv.cols)
	}
	if hv.mat[1]+hv.mat[3] != 4 {
		t.Fatalf("hist counts=%v", []float64{hv.mat[1], hv.mat[3]})
	}

	kv, ok, err := builtinCallStats(e, "convolve", []Value{ArrayValue([]float64{1, 2}), ArrayValue([]float64{3, 4})})
	if !ok || err != nil {
		t.Fatalf("convolve ok=%v err=%v", ok, err)
	}
	if kv.kind != valueArray || len(kv.arr) != 3 || kv.arr[0] != 3 || kv.arr[1] != 10 || kv.arr[2] != 8 {
		t.Fatalf("convolve=%v kind=%v", kv.arr, kv.kind)
	}
}
