// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// Selecting a frame with `.sh.level` moves the **variable scope** there too,
// so a function that selects its caller reads the caller's locals rather than
// its own. That is the other half of what a debug trap wants: the name of the
// frame it is looking at, and the variables standing in it.
//
// Measured on AT&T ksh93u+ 2012-08-01 (`/bin/ksh`, macOS 25.6) 2026-09-20,
// from the script below run as a file:
//
//	A base v=[I]
//	B lvl1 v=[O]
//	C lvl1 only_outer=[Y]
//	D lvl1 only_inner=[UNSET]
//	E lvl2 v=[I]
//	F lvl0 v=[UNSET]
//	G lvl0 g=[G]
//	H lvl0 only_outer=[UNSET]
//	I after v=[O]
//
// Rows C and D are the discriminating pair, and the reason a test that reads
// one name proves nothing: a name **both** frames hold could read right by
// accident from either scope. C is a name only the caller has and D a name
// only the callee has, so a selection that did nothing would fail D and a
// selection that merely *added* the caller's names would fail D as well.
//
// Row F says level 0 is the globals and not "no locals in the way": `v` is a
// local of both functions and nothing at the top level, so it reads unset
// there while the genuinely global `g` of row G still reads.
//
// Row I is the control that the move is a *view* and not a copy: the callee's
// own `v` is untouched by having been read through, and the caller's is what
// it always was.
func TestSelectingAFrameMovesTheVariableScope(t *testing.T) {
	dir := t.TempDir()
	src := strings.Join([]string{
		`g=G`,
		`function inner {`,
		`  typeset v=I`,
		`  typeset only_inner=X`,
		`  printf "A base v=[%s]\n" "$v"`,
		`  .sh.level=1`,
		`  printf "B lvl1 v=[%s]\n" "$v"`,
		`  printf "C lvl1 only_outer=[%s]\n" "${only_outer-UNSET}"`,
		`  printf "D lvl1 only_inner=[%s]\n" "${only_inner-UNSET}"`,
		`  .sh.level=2`,
		`  printf "E lvl2 v=[%s]\n" "$v"`,
		`  .sh.level=0`,
		`  printf "F lvl0 v=[%s]\n" "${v-UNSET}"`,
		`  printf "G lvl0 g=[%s]\n" "$g"`,
		`  printf "H lvl0 only_outer=[%s]\n" "${only_outer-UNSET}"`,
		`}`,
		`function outer {`,
		`  typeset v=O`,
		`  typeset only_outer=Y`,
		`  inner`,
		`  printf "I after v=[%s]\n" "$v"`,
		`}`,
		`outer`,
	}, "\n")
	out, st := runKsh(t, dir, src)
	if st != 0 {
		t.Fatalf("status = %d, want 0 — output %q", st, out)
	}
	want := strings.Join([]string{
		"A base v=[I]",
		"B lvl1 v=[O]",
		"C lvl1 only_outer=[Y]",
		"D lvl1 only_inner=[UNSET]",
		"E lvl2 v=[I]",
		"F lvl0 v=[UNSET]",
		"G lvl0 g=[G]",
		"H lvl0 only_outer=[UNSET]",
		"I after v=[O]",
	}, "\n") + "\n"
	if out != want {
		t.Errorf("output:\n%s\nwant:\n%s", out, want)
	}
}

// A **write** goes to the selected frame as well, which is the half that says
// the selection moved the scope rather than arranging a read-through.
//
// Measured on the same ksh93u+ and the same day:
//
//	W1 at1 v=[WROTE]
//	U1 at1 set=[WROTE]
//	V0 at0 v=[UNSET]
//	W2 back v=[I]
//	AFTER outer v=[WROTE]
//	TOP newglob=[NG] v=[UNSET]
//
// Row W2 is the discriminating one. The write at level 1 landed in the
// caller's cell and *not* in the callee's, so stepping back to the callee's
// own level reads what it always held; a write that had gone to the innermost
// scope would read `WROTE` there and `O` at the caller. Row AFTER is the same
// claim from the other side: the caller sees the write after the callee has
// returned, so it was not undone by the unwind.
//
// Row TOP says a write at level 0 to a name no frame has shadowed is a plain
// global, and that `v` — which every frame shadowed — is still gone once they
// have all returned.
func TestAWriteGoesToTheSelectedFrame(t *testing.T) {
	dir := t.TempDir()
	src := strings.Join([]string{
		`function inner {`,
		`  typeset v=I`,
		`  .sh.level=1`,
		`  v=WROTE`,
		`  printf "W1 at1 v=[%s]\n" "$v"`,
		`  printf "U1 at1 set=[%s]\n" "${v-UNSET}"`,
		`  .sh.level=0`,
		`  printf "V0 at0 v=[%s]\n" "${v-UNSET}"`,
		`  newglob=NG`,
		`  .sh.level=2`,
		`  printf "W2 back v=[%s]\n" "$v"`,
		`}`,
		`function outer {`,
		`  typeset v=O`,
		`  inner`,
		`  printf "AFTER outer v=[%s]\n" "$v"`,
		`}`,
		`outer`,
		`printf "TOP newglob=[%s] v=[%s]\n" "${newglob-UNSET}" "${v-UNSET}"`,
	}, "\n")
	out, st := runKsh(t, dir, src)
	if st != 0 {
		t.Fatalf("status = %d, want 0 — output %q", st, out)
	}
	want := strings.Join([]string{
		"W1 at1 v=[WROTE]",
		"U1 at1 set=[WROTE]",
		"V0 at0 v=[UNSET]",
		"W2 back v=[I]",
		"AFTER outer v=[WROTE]",
		"TOP newglob=[NG] v=[UNSET]",
	}, "\n") + "\n"
	if out != want {
		t.Errorf("output:\n%s\nwant:\n%s", out, want)
	}
}
