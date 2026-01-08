package vector

import (
	"testing"

	"spark/sparkos/kernel"
)

func TestProject3DToPlot_ZGoesUp(t *testing.T) {
	tk := New(nil, kernel.Capability{}, kernel.Capability{})
	tk.plotDim = 3
	tk.plotYaw = 0
	tk.plotPitch = 0.8
	tk.plotZoom = 1

	plotW := int16(200)
	plotH := int16(160)

	_, py0, _, ok := tk.project3DToPlot(0, 0, 0, plotW, plotH)
	if !ok {
		t.Fatalf("project origin failed")
	}
	_, pyUp, _, ok := tk.project3DToPlot(0, 0, 1, plotW, plotH)
	if !ok {
		t.Fatalf("project +Z failed")
	}
	_, pyDown, _, ok := tk.project3DToPlot(0, 0, -1, plotW, plotH)
	if !ok {
		t.Fatalf("project -Z failed")
	}

	// In screen coordinates, smaller y means "up".
	if !(pyUp < py0 && py0 < pyDown) {
		t.Fatalf("expected +Z up: pyUp=%v py0=%v pyDown=%v", pyUp, py0, pyDown)
	}
}
