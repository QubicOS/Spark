package shell

import "testing"

func TestInitTabsIfNeeded_DoesNotClobberState(t *testing.T) {
	s := &Service{
		cwd:  "/work",
		line: []rune("echo hi"),
	}
	s.initTabsIfNeeded()
	if got := string(s.line); got != "echo hi" {
		t.Fatalf("line changed after initTabsIfNeeded: %q", got)
	}
	if len(s.tabs) != 1 {
		t.Fatalf("tabs=%d, want 1", len(s.tabs))
	}
	if s.tabs[0].cwd != "/work" {
		t.Fatalf("tab cwd=%q, want /work", s.tabs[0].cwd)
	}
	if got := string(s.tabs[0].line); got != "echo hi" {
		t.Fatalf("tab line=%q, want %q", got, "echo hi")
	}
}
