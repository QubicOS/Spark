package shell

import "testing"

func TestParseEscape_Arrows(t *testing.T) {
	tcs := []struct {
		in   string
		n    int
		act  escAction
		ok   bool
		name string
	}{
		{name: "up", in: "\x1b[A", n: 3, act: escUp, ok: true},
		{name: "down", in: "\x1b[B", n: 3, act: escDown, ok: true},
		{name: "right", in: "\x1b[C", n: 3, act: escRight, ok: true},
		{name: "left", in: "\x1b[D", n: 3, act: escLeft, ok: true},
		{name: "home", in: "\x1b[H", n: 3, act: escHome, ok: true},
		{name: "end", in: "\x1b[F", n: 3, act: escEnd, ok: true},
		{name: "delete", in: "\x1b[3~", n: 4, act: escDelete, ok: true},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			n, act, ok := parseEscape([]byte(tc.in))
			if ok != tc.ok || n != tc.n || act != tc.act {
				t.Fatalf("parseEscape(%q) = n=%d act=%v ok=%v; want n=%d act=%v ok=%v", tc.in, n, act, ok, tc.n, tc.act, tc.ok)
			}
		})
	}
}

func TestParseEscape_FKeys(t *testing.T) {
	tcs := []struct {
		in   string
		n    int
		act  escAction
		ok   bool
		name string
	}{
		{name: "f1", in: "\x1b[11~", n: 5, act: escF1, ok: true},
		{name: "f2", in: "\x1b[12~", n: 5, act: escF2, ok: true},
		{name: "f3", in: "\x1b[13~", n: 5, act: escF3, ok: true},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			n, act, ok := parseEscape([]byte(tc.in))
			if ok != tc.ok || n != tc.n || act != tc.act {
				t.Fatalf("parseEscape(%q) = n=%d act=%v ok=%v; want n=%d act=%v ok=%v", tc.in, n, act, ok, tc.n, tc.act, tc.ok)
			}
		})
	}
}
