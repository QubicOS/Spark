package vector

import "testing"

func TestParseInput_FunctionCallsAndDefinitions(t *testing.T) {
	tests := []struct {
		in       string
		wantKind actionKind
	}{
		{in: "sin(x)", wantKind: actionEval},
		{in: "simp(sin(x)*cos(y))", wantKind: actionEval},
		{in: "diff(sin(x), x)", wantKind: actionEval},
		{in: "f(x)=x", wantKind: actionAssignFunc},
		{in: "f(x)", wantKind: actionEval},
		{in: "(sin(x))", wantKind: actionEval},
		{in: "sin((x))", wantKind: actionEval},
	}

	for _, tt := range tests {
		acts, err := parseInput(tt.in)
		if err != nil {
			t.Fatalf("parseInput(%q) error: %v", tt.in, err)
		}
		if len(acts) != 1 {
			t.Fatalf("parseInput(%q) actions=%d, want 1", tt.in, len(acts))
		}
		if acts[0].kind != tt.wantKind {
			t.Fatalf("parseInput(%q) kind=%v, want %v", tt.in, acts[0].kind, tt.wantKind)
		}
	}
}
