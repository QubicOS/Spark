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

func TestPlotKey_TabToggles3DAxes(t *testing.T) {
	tk := New(nil, kernel.Capability{}, kernel.Capability{})
	tk.tab = tabPlot
	tk.plotDim = 3

	if tk.showAxes3D {
		t.Fatalf("expected default showAxes3D=false")
	}

	tk.handleKey(nil, key{kind: keyTab})
	if !tk.showAxes3D {
		t.Fatalf("expected showAxes3D=true after Tab")
	}
	tk.handleKey(nil, key{kind: keyTab})
	if tk.showAxes3D {
		t.Fatalf("expected showAxes3D=false after second Tab")
	}
}

func TestREPL_AllowsRuneHotkeys(t *testing.T) {
	tk := New(nil, kernel.Capability{}, kernel.Capability{})
	tk.tab = tabTerminal

	tk.handleKey(nil, key{kind: keyRune, r: 'q'})
	tk.handleKey(nil, key{kind: keyRune, r: 'g'})
	tk.handleKey(nil, key{kind: keyRune, r: 'h'})

	got := string(tk.input)
	if got != "qgh" {
		t.Fatalf("input=%q", got)
	}
}
