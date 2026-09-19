// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// A `-g` declaration written inside a call that was given an assignment
// prefix. The prefix and the letter each have an answer already — the prefix
// is given back when the call ends, and the letter says the declaration takes
// no scope of its own — and where the write lands is a third thing neither of
// them says.
//
// It is Semantics.DeclareGlobalReachesPastALocal reached through the other
// mechanism and not a second axis: the letter either names the shell's own
// cell, in which case it writes under whatever is standing on the name, or it
// means "take no new local", in which case it writes the binding that is
// visible — which here is the prefix's, and which leaves with it.
//
// Named for the axis and never for a shell, as everything in this package is.

// globalLetter is the vector these tests share: a declaration utility with
// the `-g` letter, under each answer to where the letter writes.
func globalLetter(reaches Answer) func(*Semantics) {
	return func(s *Semantics) {
		s.DeclareOptions = "aAgiprx"
		s.LocalOptions = "aAiprx"
		s.DeclareGlobalReachesPastALocal = reaches
		// Whether a `typeset` inside a function declares a local at all is
		// a question of its own, answered here so the control row reaches
		// this one.
		s.TypesetLocalNeedsKeywordFunction = No
	}
}

// The shape the axis is named for, and the probe that says it is a stack
// rather than a keep: the inner call gives back what it displaced and the
// outer one gives back what the declaration wrote, so the write went
// underneath both temporary bindings rather than over either.
func TestAGlobalDeclarationWritesUnderACallsAssignmentPrefix(t *testing.T) {
	const src = `inner() { typeset -g t=3; echo "1[${t-U}]"; }
outer() { t=7 inner; echo "2[${t-U}]"; }
t=9 outer; echo "3[${t-U}]"`
	for _, tc := range []struct {
		reaches Answer
		want    string
	}{
		// The read inside the body is still the prefix's, under either
		// answer for the column that writes underneath and for the reason
		// the other column reads `3`: there the declaration wrote the
		// visible binding.
		{Yes, "1[7]\n2[9]\n3[3]\n"},
		{No, "1[3]\n2[9]\n3[U]\n"},
	} {
		out, errs, st := declRun(t, src, globalLetter(tc.reaches), Diagnostics{})
		if out != tc.want || st != 0 || errs != "" {
			t.Errorf("%v = %q (stderr %q, status %d), want %q",
				tc.reaches, out, errs, st, tc.want)
		}
	}
}

// How far the write reaches, and how far it does not. It is the **outermost**
// prefix frame holding the name that is written — three calls deep, each
// take-back still gives back the next one out — and it is reached from a
// callee with no prefix of its own. A name the prefix was never holding is
// the control: the declaration writes the shell's cell as it always did.
func TestAGlobalDeclarationReachesTheOutermostPrefixHoldingTheName(t *testing.T) {
	const src = `a3() { typeset -g v=3; }
b3() { v=7 a3; echo "1[${v-U}]"; }
c3() { v=8 b3; echo "2[${v-U}]"; }
v=9 c3; echo "3[${v-U}]"
e() { typeset -g w=3; }
e2() { e; }
w=7 e2; echo "4[${w-U}]"
d() { typeset -g y=3; }
z=7 d; echo "5[${y-U}][${z-U}]"`
	for _, tc := range []struct {
		reaches Answer
		want    string
	}{
		{Yes, "1[8]\n2[9]\n3[3]\n4[3]\n5[3][U]\n"},
		{No, "1[8]\n2[9]\n3[U]\n4[U]\n5[3][U]\n"},
	} {
		out, errs, st := declRun(t, src, globalLetter(tc.reaches), Diagnostics{})
		if out != tc.want || st != 0 || errs != "" {
			t.Errorf("%v = %q (stderr %q, status %d), want %q",
				tc.reaches, out, errs, st, tc.want)
		}
	}
}

// A `local` and a prefix over the same name, both orders. Which of the two
// holds the shell's own cell is decided by which took the name first, and
// the two orders answer differently: with the scope outside the frame the
// declaration writes the scope's saved copy, and with the frame outside the
// scope it writes the frame's.
//
// Both rows end at `3` under the answer that reaches, which is what makes
// them worth having: they get there through different cells, and reading the
// body is what tells them apart.
func TestWhichOfALocalAndACallsPrefixHoldsTheShellsCell(t *testing.T) {
	const src = `g1() { typeset -g q=3; }
f1() { local q=5; q=7 g1; echo "1[${q-U}]"; }
q=1; f1; echo "2[${q-U}]"
g2() { local r=5; typeset -g r=3; echo "3[${r-U}]"; }
r=9 g2; echo "4[${r-U}]"`
	for _, tc := range []struct {
		reaches Answer
		want    string
	}{
		{Yes, "1[5]\n2[3]\n3[5]\n4[3]\n"},
		{No, "1[5]\n2[1]\n3[3]\n4[U]\n"},
	} {
		out, errs, st := declRun(t, src, globalLetter(tc.reaches), Diagnostics{})
		if out != tc.want || st != 0 || errs != "" {
			t.Errorf("%v = %q (stderr %q, status %d), want %q",
				tc.reaches, out, errs, st, tc.want)
		}
	}
}

// It is not the value alone that goes underneath. The export letter and the
// freeze on the same line land on the shell's own cell too, which is why the
// bindings come off around the whole declaration rather than around the
// store.
func TestAGlobalDeclarationsAttributesGoUnderTheCallsPrefixToo(t *testing.T) {
	const src = `f() { typeset -gx e=3; }
e=7 f; typeset -p e
g() { typeset -gr n=3; }
n=7 g; typeset -p n`
	out, errs, st := declRun(t, src, globalLetter(Yes), Diagnostics{})
	const want = "declare -x e=\"3\"\ndeclare -r n=\"3\"\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("= %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// The controls, which are what keep this from being "a declaration wins". A
// valueless `-g` writes nothing and is taken back with everything else, a
// declaration with no `-g` is the ordinary local, and a plain assignment to
// the same name inside the body writes the binding the prefix put there and
// goes back with it.
func TestWhatACallsPrefixStillTakesBack(t *testing.T) {
	const src = `f() { typeset -g a; }
a=7 f; echo "1[${a-U}]"
g() { typeset b=3; }
b=7 g; echo "2[${b-U}]"
h() { c=3; }
c=7 h; echo "3[${c-U}]"
i() { typeset -g d=3; d=8; }
d=7 i; echo "4[${d-U}]"`
	for _, tc := range []struct {
		reaches Answer
		want    string
	}{
		// Row 4 is the one the answers split on, and it splits the same way
		// the first test's last row does: the `-g` write is underneath and
		// the plain `d=8` is on top of it, so what comes back is the 3.
		{Yes, "1[U]\n2[U]\n3[U]\n4[3]\n"},
		{No, "1[U]\n2[U]\n3[U]\n4[U]\n"},
	} {
		out, errs, st := declRun(t, src, globalLetter(tc.reaches), Diagnostics{})
		if out != tc.want || st != 0 || errs != "" {
			t.Errorf("%v = %q (stderr %q, status %d), want %q",
				tc.reaches, out, errs, st, tc.want)
		}
	}
}
