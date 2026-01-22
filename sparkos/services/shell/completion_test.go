package shell

import "testing"

func TestUpdateCompletion_CommandName(t *testing.T) {
	s := &Service{}
	if err := s.initRegistry(); err != nil {
		t.Fatalf("initRegistry: %v", err)
	}

	s.line = []rune("foc")
	s.cursor = len(s.line)
	s.updateCompletion()

	if s.best != "focus" {
		t.Fatalf("best=%q, want %q", s.best, "focus")
	}
	if s.ghost != "us" {
		t.Fatalf("ghost=%q, want %q", s.ghost, "us")
	}
	if s.compMode != completionCommand {
		t.Fatalf("compMode=%v, want %v", s.compMode, completionCommand)
	}
	if s.compToken != "foc" {
		t.Fatalf("compToken=%q, want %q", s.compToken, "foc")
	}
}

func TestUpdateCompletion_ArgValue(t *testing.T) {
	s := &Service{}
	if err := s.initRegistry(); err != nil {
		t.Fatalf("initRegistry: %v", err)
	}

	s.line = []rune("focus a")
	s.cursor = len(s.line)
	s.updateCompletion()

	if s.compMode != completionArg {
		t.Fatalf("compMode=%v, want %v", s.compMode, completionArg)
	}
	if s.best != "app" {
		t.Fatalf("best=%q, want %q", s.best, "app")
	}
	if s.ghost != "pp" {
		t.Fatalf("ghost=%q, want %q", s.ghost, "pp")
	}
	if len(s.cands) != 1 {
		t.Fatalf("cands=%d, want 1", len(s.cands))
	}
}

func TestUpdateCompletion_HelpCompletesCommands(t *testing.T) {
	s := &Service{}
	if err := s.initRegistry(); err != nil {
		t.Fatalf("initRegistry: %v", err)
	}

	s.line = []rune("help fo")
	s.cursor = len(s.line)
	s.updateCompletion()

	if s.compMode != completionArg {
		t.Fatalf("compMode=%v, want %v", s.compMode, completionArg)
	}
	if s.best != "focus" {
		t.Fatalf("best=%q, want %q", s.best, "focus")
	}
	if s.ghost != "cus" {
		t.Fatalf("ghost=%q, want %q", s.ghost, "cus")
	}
}
