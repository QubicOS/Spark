package shell

import "testing"

func TestParseArgs_EscapesAndQuotes(t *testing.T) {
	tcs := []struct {
		line string
		args []string
		ok   bool
	}{
		{line: `echo "a\"b"`, args: []string{"echo", `a"b`}, ok: true},
		{line: `echo "a\ b"`, args: []string{"echo", "a b"}, ok: true},
		{line: `echo foo\ bar`, args: []string{"echo", "foo bar"}, ok: true},
		{line: `echo "oops`, args: nil, ok: false},
	}

	for _, tc := range tcs {
		args, redir, ok := parseArgs(tc.line)
		if ok != tc.ok {
			t.Fatalf("parseArgs(%q) ok=%v; want %v", tc.line, ok, tc.ok)
		}
		if !ok {
			continue
		}
		if redir.Path != "" || redir.Append {
			t.Fatalf("parseArgs(%q) redir=%+v; want empty", tc.line, redir)
		}
		if len(args) != len(tc.args) {
			t.Fatalf("parseArgs(%q) args=%v; want %v", tc.line, args, tc.args)
		}
		for i := range args {
			if args[i] != tc.args[i] {
				t.Fatalf("parseArgs(%q) args[%d]=%q; want %q", tc.line, i, args[i], tc.args[i])
			}
		}
	}
}
