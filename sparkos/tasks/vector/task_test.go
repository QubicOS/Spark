package vector

import (
	"testing"

	"spark/sparkos/kernel"
)

func TestEvalLine_SetsGraphFor3DSurface(t *testing.T) {
	tk := New(nil, kernel.Capability{}, kernel.Capability{})
	tk.plotDim = 3

	tk.e.vars["x"] = ArrayValue([]float64{-1, 0, 1})
	tk.e.vars["y"] = ArrayValue([]float64{-1, 0, 1})

	tk.evalLine(nil, "simp(sin(x)*cos(y))", false)

	if tk.graph == nil || tk.graphExpr == "" {
		t.Fatalf("graph not set for 3D surface expression")
	}
}
