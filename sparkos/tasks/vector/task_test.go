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

func TestEvalLine_AssignYPlotsAgainstX(t *testing.T) {
	tk := New(nil, kernel.Capability{}, kernel.Capability{})

	// Fresh notebook should have x seeded so this assignment works and creates a plot.
	tk.evalLine(nil, "y = x", false)

	if len(tk.plots) == 0 {
		t.Fatalf("expected y=x to create a plot series")
	}
}

func TestService_PlotColorCycles(t *testing.T) {
	tk := New(nil, kernel.Capability{}, kernel.Capability{})
	before := tk.plotColorMode
	tk.handleServiceCommand(nil, "plotcolor")
	if tk.plotColorMode == before {
		t.Fatalf("plotcolor did not change mode")
	}
	tk.handleServiceCommand(nil, "plotcolor mono")
	if tk.plotColorMode != 0 {
		t.Fatalf("plotcolor mono=%d", tk.plotColorMode)
	}
	tk.handleServiceCommand(nil, "plotcolor height")
	if tk.plotColorMode != 1 {
		t.Fatalf("plotcolor height=%d", tk.plotColorMode)
	}
	tk.handleServiceCommand(nil, "plotcolor pos")
	if tk.plotColorMode != 2 {
		t.Fatalf("plotcolor pos=%d", tk.plotColorMode)
	}
}
