// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
)

// TestAKindLetterOverZeroIsRefused pins #6740: `0` is a slot of a fixed scalar
// kind, so a declaration asking it to be an array, an association, an integer
// or a float is refused with the kind sentence and ends the shell at 1 on both
// routes, as the same letters over `HOME` are. The plus letters and the
// letters that are not a kind are taken. Measured 2026-10-16 against zsh 5.9.2
// (`/opt/homebrew/bin/zsh -f` under `env -i PATH=/usr/bin:/bin`, from `-c`
// and from a script file).
func TestAKindLetterOverZeroIsRefused(t *testing.T) {
	t.Parallel()
	kind := func(loc string) string {
		return loc + " 0: can't change type of a special parameter\n"
	}
	for _, tc := range []struct{ src, want string }{
		{"typeset -A 0", kind("zsh:typeset:1:")},
		{"typeset -a 0", kind("zsh:typeset:1:")},
		{"typeset -ga 0", kind("zsh:typeset:1:")},
		{"typeset -i10 0", kind("zsh:typeset:1:")},
		{"typeset -h -a 0", kind("zsh:typeset:1:")},
		{"f(){ local -A 0; echo in; }; f", kind("f:local:")},
		{"f(){ local -a 0; echo in; }; f", kind("f:local:")},
		{"f(){ integer 0; echo in; }; f", kind("f:integer:")},
		{"f(){ float 0; echo in; }; f", kind("f:float:")},
	} {
		for _, route := range []interp.Route{interp.RouteCommandString, interp.RouteScriptFile} {
			out, st := runZshRoute(t, t.TempDir(), tc.src+"; echo after", route)
			if st != 1 {
				t.Errorf("%s from %v = status %d, want 1", tc.src, route, st)
			}
			if route == interp.RouteCommandString && out != tc.want {
				t.Errorf("%s = %q, want %q and nothing after it", tc.src, out, tc.want)
			}
		}
	}
	// The controls: what the slot takes, so that the rows above are a
	// refusal of the letter and not of the name.
	for _, tc := range []struct{ src, want string }{
		{`typeset +a 0; echo $? ${(t)0}`, "0 scalar-special\n"},
		{`typeset +A 0; echo $? ${(t)0}`, "0 scalar-special\n"},
		{`typeset -r 0; echo $? ${(t)0}`, "0 scalar-readonly-special\n"},
		{`typeset -x 0; echo $? ${(t)0}`, "0 scalar-export-special\n"},
		{`f(){ local 0; echo $? ${(t)0}; }; f`, "0 scalar-local-special\n"},
		{`f(){ local -h -a 0; echo $?; }; f`, "0\n"},
	} {
		out, _ := runZshRoute(t, t.TempDir(), tc.src, interp.RouteCommandString)
		if out != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
		}
	}
}

// TestARefusedReadArrayAtEndOfInputLeavesTheReadsStatus pins #6744: a `read
// -A` whose whole-array store is refused ends the script, and from `-c` the
// number left is the read's own — 1 where it hit end of input — rather than
// the 0 a refused `set -A` leaves. All four refusals answer alike. Measured
// 2026-10-16 against zsh 5.9.2, `-f`, `env -i PATH=/usr/bin:/bin`.
func TestARefusedReadArrayAtEndOfInputLeavesTheReadsStatus(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		src  string
		want int
	}{
		{"read -A HOME </dev/null", 1},
		{"read -A 0 </dev/null", 1},
		{"print -n x | read -A HOME", 1},
		{"typeset -r v; read -A v </dev/null", 1},
		{"f(){ private v; g; }; g(){ read -A v </dev/null; }; f", 1},
		// The controls: a read that got its line, and `set -A`.
		{"read -A HOME <<< x", 0},
		{"read -A 0 <<< x", 0},
		{"typeset -r v; read -A v <<< x", 0},
		{"f(){ private v; g; }; g(){ read -A v <<< x; }; f", 0},
		{"set -A HOME x", 0},
	} {
		out, st := runZshRoute(t, t.TempDir(), tc.src+"; echo after", interp.RouteCommandString)
		if st != tc.want {
			t.Errorf("%s = status %d, want %d", tc.src, st, tc.want)
		}
		if strings.Contains(out, "after") {
			t.Errorf("%s went on: %q", tc.src, out)
		}
		if _, st := runZshRoute(t, t.TempDir(), tc.src, interp.RouteScriptFile); st != 1 {
			t.Errorf("%s from a script file = status %d, want 1", tc.src, st)
		}
	}
	const sub = "( read -A HOME </dev/null ); echo $?"
	out, _ := runZshRoute(t, t.TempDir(), sub, interp.RouteCommandString)
	if want := "zsh:1: HOME: attempt to assign array value to non-array\n1\n"; out != want {
		t.Errorf("%s = %q, want %q", sub, out, want)
	}
}

// TestAPositionTooWideWarnsAtEveryStore pins #6743: an index too wide for a
// signed 64-bit integer warns `number truncated after N digits` once per
// store, after the value is expanded, on every route that stores into a
// position, and the store is still declined (see
// Runner.warnATruncatedPosition for why). Measured 2026-10-16 against zsh
// 5.9.2, `-f`, `env -i PATH=/usr/bin:/bin`, from `-c` and a script file.
func TestAPositionTooWideWarnsAtEveryStore(t *testing.T) {
	t.Parallel()
	const n = "99999999999999999999"
	const warn = "zsh:1: number truncated after 19 digits: " + n + "\n"
	for _, tc := range []struct {
		src   string
		warns int
	}{
		{"set -- a b; " + n + "=x", 1},
		{"set -- a b; " + n + "=(x y)", 1},
		{"set -- a b; " + n + "+=x", 1},
		{"set -- a b; set -A " + n + " x", 1},
		{"set -- a b; read " + n + " <<< x", 1},
		{"set -- a b; read -A " + n + " <<< x", 1},
		{"set -- a b; printf -v " + n + " x", 2},
		{"set -- a b; for " + n + " in x y; do :; done", 2},
		{"set -- a b; f(){ :; }; " + n + "=x f", 1},
		{"set -- a b; " + n + "=x true", 1},
		{"set -- a b; " + n + "=x " + n + "=y", 2},
		{"set -- a b; " + n + "=x /usr/bin/true", 1},
		{"set -- a b; zparseopts -a " + n + " a", 1},
	} {
		out, st := runZshOnPath(t, t.TempDir(), tc.src+`; echo $? "[$*]"`)
		if want := strings.Repeat(warn, tc.warns) + "0 [a b]\n"; out != want || st != 0 {
			t.Errorf("%s = %q (%d), want %q (0)", tc.src, out, st, want)
		}
	}
	// The value is expanded ahead of the warning.
	const side = "set -- a; " + n + "=$(echo side) /usr/bin/true"
	if out, _ := runZshOnPath(t, t.TempDir(), side); out != warn {
		t.Errorf("%s = %q, want only the warning", side, out)
	}
	const order = n + "=$(echo side >&2)"
	if out, _ := runZshOnPath(t, t.TempDir(), order); out != "side\n"+warn {
		t.Errorf("%s = %q, want side then the warning", order, out)
	}
}

// TestTheTruncatedDigitCountIsTheShells pins the count the warning gives,
// which is one past the digit that went over unless that digit was the last,
// and is shared with a numbered tilde. Measured 2026-10-16 against zsh 5.9.2,
// `-f` (#6743).
func TestTheTruncatedDigitCountIsTheShells(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		index string
		count int
	}{
		{"9223372036854775808", 18},
		{"9999999999999999999", 18},
		{"10000000000000000000", 19},
		{"92233720368547758070", 19},
		{"99999999999999999999", 19},
		{"999999999999999999999", 19},
		{"100000000000000000000", 20},
		{"1000000000000000000000", 20},
		{"00000000000000000003", 0},
		{"9223372036854775807", 0},
	} {
		src := "set -- a; " + tc.index + "=x 2>&1"
		want := ""
		if tc.count > 0 {
			want = "zsh:1: number truncated after " + itoa(int64(tc.count)) + " digits: " + tc.index + "\n"
		}
		if out, _ := runZshOnPath(t, t.TempDir(), src); out != want {
			t.Errorf("%s = %q, want %q", src, out, want)
		}
		if tc.count == 0 {
			continue
		}
		tilde := "echo ~" + tc.index + "/x"
		out, _ := runZshOnPath(t, t.TempDir(), tilde)
		if prefix := "zsh:1: number truncated after " + itoa(
			int64(tc.count),
		) + " digits: " + tc.index + "/x\n"; !strings.HasPrefix(
			out,
			prefix,
		) {
			t.Errorf("%s = %q, want it to begin %q", tilde, out, prefix)
		}
	}
}
