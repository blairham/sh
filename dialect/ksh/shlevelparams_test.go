// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// Selecting a frame with `.sh.level` moves the **positional parameters and
// `$0`** there as well as the variables, so each level answers with that
// frame's own argument list. The third thing the selection moves, after the
// two location parameters and the variable scope (#3952).
//
// Measured on AT&T ksh93u+ 2012-08-01 (`/bin/ksh`, macOS 25.6) 2026-09-20,
// from the script below run as a file with **no arguments**:
//
//	A lvl1 1=[1] 2=[2] hash=[3] star=[1 2 3] at=[<1><2><3>]
//	B lvl0 1=[UNSET] hash=[0] star=[]
//	C lvl2 1=[a] hash=[3] star=[a b c] at=[<a><b><c>]
//	D back 1=[a] hash=[3]
//
// **Level 0 is the discriminating row, and the empty argument list is why.**
// A selection that did nothing would answer the callee's 3 there; one that
// merely reached the *caller* would answer the caller's 3 as well. Only the
// script's own list is 0, and running the same script with two arguments
// answers those two at level 0 — so it is that frame's list rather than
// emptiness.
//
// Row D is the control that the move is a *view*: stepping back to the
// frame the shell is standing in reads what it always held.
func TestSelectingAFrameMovesThePositionalParameters(t *testing.T) {
	src := strings.Join([]string{
		`function inner {`,
		`  .sh.level=1`,
		`  printf "A lvl1 1=[%s] 2=[%s] hash=[%s] star=[%s] at=[" "${1-UNSET}" "${2-UNSET}" "$#" "$*"`,
		`  printf "<%s>" "$@"`,
		`  printf "]\n"`,
		`  .sh.level=0`,
		`  printf "B lvl0 1=[%s] hash=[%s] star=[%s]\n" "${1-UNSET}" "$#" "$*"`,
		`  .sh.level=2`,
		`  printf "C lvl2 1=[%s] hash=[%s] star=[%s] at=[" "${1-UNSET}" "$#" "$*"`,
		`  printf "<%s>" "$@"`,
		`  printf "]\n"`,
		`  printf "D back 1=[%s] hash=[%s]\n" "${1-UNSET}" "$#"`,
		`}`,
		`function outer { inner a b c; }`,
		`outer 1 2 3`,
	}, "\n")
	out, st := runKsh(t, t.TempDir(), src)
	if st != 0 {
		t.Fatalf("status = %d, want 0 — output %q", st, out)
	}
	want := strings.Join([]string{
		"A lvl1 1=[1] 2=[2] hash=[3] star=[1 2 3] at=[<1><2><3>]",
		"B lvl0 1=[UNSET] hash=[0] star=[]",
		"C lvl2 1=[a] hash=[3] star=[a b c] at=[<a><b><c>]",
		"D back 1=[a] hash=[3]",
		"",
	}, "\n")
	if out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}

// `$0` moves with them, and it is the ordinary rule asked of a **shorter
// stack** rather than the selected frame's own name. This shell's `$0` names
// the innermost function defined with the `function` keyword, and a `name()`
// frame is transparent to that walk with a selection exactly as without one.
//
// Measured 2026-09-20 with `function A` calling `B()` calling `function C`,
// from a script file:
//
//	L3 zero=[C]        the frame the shell is standing in
//	L2 zero=[A]        B is a `name()` function, so the walk goes past it
//	L1 zero=[A]
//	L0 zero=[<the shell's own name>]
//
// Row L2 is the discriminator. Reading the frame at the level would have
// answered with the shell's own name there, since `B` is not a keyword
// function and has no `$0` of its own to give.
func TestSelectingAFrameMovesDollarZeroThroughTheOrdinaryRule(t *testing.T) {
	src := strings.Join([]string{
		`function C {`,
		`  .sh.level=3; printf "L3 zero=[%s]\n" "$0"`,
		`  .sh.level=2; printf "L2 zero=[%s]\n" "$0"`,
		`  .sh.level=1; printf "L1 zero=[%s]\n" "$0"`,
		`  .sh.level=0; printf "L0 zero=[%s]\n" "$0"`,
		`}`,
		`B() { C c1; }`,
		`function A { B b1; }`,
		`A a1`,
	}, "\n")
	out, st := runKsh(t, t.TempDir(), src)
	if st != 0 {
		t.Fatalf("status = %d, want 0 — output %q", st, out)
	}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("got %q, want four lines", out)
	}
	for i, want := range []string{"L3 zero=[C]", "L2 zero=[A]", "L1 zero=[A]"} {
		if lines[i] != want {
			t.Errorf("line %d = %q, want %q", i+1, lines[i], want)
		}
	}
	// Level 0 is below every function, so it is whatever `$0` is with
	// nothing called — the shell's own name here, and a script's path when
	// there is a script. Asserted against the other rows rather than against
	// a literal, because the string is the invocation's rather than the
	// selection's.
	if lines[3] == "L0 zero=[A]" || lines[3] == "L0 zero=[C]" {
		t.Errorf("line 4 = %q, want the top level's own `$0` and not a function's", lines[3])
	}
}

// And the narrowing, stated as a test so that it is a decision rather than an
// omission: a **write** at a selected level is not diverted here. `set --`
// and `shift` reach the list the shell is standing in.
//
// That is a measured divergence and not a guess. Real ksh93u+ diverts the
// write too — measured 2026-09-20 from a script file, with `inner a b c`
// called from `outer 1 2 3`:
//
//	                          real ksh93u+                  here
//	.sh.level=1; shift        inner a/3, outer 2/2          inner b/2, outer 1/3
//	.sh.level=1; set -- z     inner z/3, outer z/1          inner z/1, outer 1/3
//
// The read half is a walk over records that already exist; the write half is
// a second mechanism with its own unwinding, and the reference's own answers
// above are not self-consistent enough to implement from — `inner` reads
// three parameters after a `set -- z` that left one. So it is left where it
// was and measured rather than half-answered, exactly as the variable half
// left `unset` and a declaration alone. See #3952.
func TestAWriteAtASelectedLevelStillReachesTheRunningFrame(t *testing.T) {
	src := strings.Join([]string{
		`function inner {`,
		`  .sh.level=1`,
		`  shift`,
		`  .sh.level=2`,
		`  printf "inner 1=[%s] hash=[%s]\n" "${1-UNSET}" "$#"`,
		`}`,
		`function outer { inner a b c; printf "outer 1=[%s] hash=[%s]\n" "${1-UNSET}" "$#"; }`,
		`outer 1 2 3`,
	}, "\n")
	out, st := runKsh(t, t.TempDir(), src)
	if st != 0 {
		t.Fatalf("status = %d, want 0 — output %q", st, out)
	}
	want := "inner 1=[b] hash=[2]\nouter 1=[1] hash=[3]\n"
	if out != want {
		t.Errorf("got %q, want %q — the shift landed in the caller's list rather "+
			"than in the running frame's, which this shell does not do yet", out, want)
	}
}
