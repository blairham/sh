// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/interp"
)

// TestWholeArrayStoreIntoAPositionSplices pins #6648: a builtin's whole-array
// store into a name that is a position is `argv[N]=(…)`, which the words
// replace, padding the list when N lies past its end. Measured 2026-10-15
// against zsh 5.9.2 (`/opt/homebrew/bin/zsh -f` under `env -i
// PATH=/usr/bin:/bin`, from `-c` and from a script file).
func TestWholeArrayStoreIntoAPositionSplices(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ src, want string }{
		{`set -- a b; set -A 1 x y; echo $? "[$*]" $#`, "0 [x y b] 3\n"},
		{`set -- a b c; set +A 2 x y; echo $? "[$*]" $#`, "0 [a x y c] 4\n"},
		{`set -- a b c; set +A 2; echo $? "[$*]" $#`, "0 [a c] 2\n"},
		{`set -- a b; set -A 1; echo $? "[$*]" $#`, "0 [b] 1\n"},
		{`set -- a b; set -A 4; echo $? "[$*]" $#`, "0 [a b ] 3\n"},
		{`set -- a b; set -A 01 x y; echo $? "[$*]" $#`, "0 [x y b] 3\n"},
		{`set -- a b; readonly argv; set -A 1 x; echo $? "[$*]"`, "0 [x b]\n"},
		{`f(){ set -A 2 p q; echo "in [$*]"; }; set -- a b; f c d; echo "out [$*]"`, "in [c p q]\nout [a b]\n"},
		{`set -- a b; read -A 1 <<< "x y"; echo $? "[$*]" $#`, "0 [x y b] 3\n"},
		{`set -- a b; read -A 4 <<< p; echo $? "[$*]" $#`, "0 [a b  p] 4\n"},
		{`set -- a b; read -A 1 < /dev/null; echo $? "[$*]" $#`, "1 [ b] 2\n"},
		{`set -- a b; read -e -A 1 <<< "x y"; echo "[$*]"`, "x\ny\n[a b]\n"},
		{`set -- -a b; zparseopts -a 2 a; echo $? "[$*]"`, "0 [-a -a]\n"},
		{`set -- a b; zparseopts -a 1 a; echo $? "[$*]"`, "0 [b]\n"},
		{`set -- -a b; zparseopts -a x a=1; echo $? "[$*]"`, "0 [-a b]\n"},
		{`set -- a b; zparseopts -A x -a 1 a; echo "[$*]" ${(t)x}`, "[b] association\n"},
		{`set -- a b; : ${(A)1::=x y}; echo $? "[$*]"`, "0 [x y b]\n"},
		{`set -- a b; print -rl -- "${(A)2::=x y}"; echo "[$*]"`, "x y\n[a x y]\n"},
		{`set -- a b; : ${(A)5::=x}; echo "[$*]" $#`, "[a b   x] 5\n"},
	} {
		out, _ := runZshOnPath(t, t.TempDir(), tc.src)
		if out != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
		}
	}
}

// TestWholeArrayStoreIntoZeroOrATableIntoAPositionIsRefused is the refusing
// half of #6648: `$0` is a slot of a fixed scalar kind, so an array is
// refused with the sentence `0=(x)` gets, and a table is refused at every
// position with the kind sentence. Each ends `-c` at 0 and a script file at
// 1, as the same builtins' refusals over `HOME` do; the expansion flag ends
// both at 1.
func TestWholeArrayStoreIntoZeroOrATableIntoAPositionIsRefused(t *testing.T) {
	t.Parallel()
	array := func(loc string) string {
		return loc + " 0: attempt to assign array value to non-array\n"
	}
	kind := func(name string) string {
		return "zsh:1: " + name + ": can't change type of a special parameter\n"
	}
	for _, tc := range []struct {
		src, want string
		cmd       int
	}{
		{"set -A 0 x", array("zsh:1:"), 0},
		{"set -A 0", array("zsh:1:"), 0},
		{"set +A 0 x", array("zsh:1:"), 0},
		{"read -A 0 <<< x", array("zsh:1:"), 0},
		{"f(){ set -A 0 x; }; f", array("f:"), 0},
		{"zparseopts -a 0 a", array("zsh:1:"), 0},
		{"set -- -a b; zparseopts -a 1 a=0", array("zsh:1:"), 0},
		{"zparseopts -a 0 -A 0 a", array("zsh:1:"), 0},
		{"zparseopts -A 1 a", kind("1"), 0},
		{"zparseopts -A 0 a", kind("0"), 0},
		{"set -- a b; zparseopts -A 01 a", kind("01"), 0},
		{": ${(A)0::=x}", array("zsh:1:"), 1},
	} {
		for _, route := range []struct {
			route interp.Route
			want  int
		}{
			{interp.RouteCommandString, tc.cmd},
			{interp.RouteScriptFile, 1},
		} {
			out, st := runZshRoute(t, t.TempDir(), tc.src+"; echo after", route.route)
			if st != route.want {
				t.Errorf("%s from %v = status %d, want %d", tc.src, route.route, st, route.want)
			}
			if route.route == interp.RouteCommandString && out != tc.want {
				t.Errorf("%s = %q, want %q and nothing after it", tc.src, out, tc.want)
			}
		}
	}
}

// TestAnOperandStoreAtAPositionPastTheBoundStoresNothing pins the bound a
// builtin's scalar store into a position shares with `N=value`: an index too
// wide to represent stores nothing, where an unbounded parse wrapped it to an
// index in the quintillions and grew the list until memory ran out. The bound
// is a resource limit and not a modeled answer — zsh 5.9.2 warns `number
// truncated after 19 digits` there and goes on to an artifact of its own index
// width (see Runner.warnATruncatedPosition) — so this asserts the warnings and
// that the shell comes back with the list untouched.
func TestAnOperandStoreAtAPositionPastTheBoundStoresNothing(t *testing.T) {
	t.Parallel()
	const src = `set -- a b; read 99999999999999999999 <<< x; printf -v 99999999999999999999 %s y; echo $? $#; read 01 <<< q; echo "[$*]"`
	out, _ := runZshOnPath(t, t.TempDir(), src)
	const warn = "zsh:1: number truncated after 19 digits: 99999999999999999999\n"
	if want := warn + warn + warn + "0 2\n[q b]\n"; out != want {
		t.Errorf("%s = %q, want %q", src, out, want)
	}
}
