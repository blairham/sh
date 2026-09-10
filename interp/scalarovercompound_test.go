// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// What a plain `a=x` does to a name that is already holding an array or a
// table: the value becomes the whole of the name, or it lands on the
// compound's first element and the rest stays where it is. See
// Semantics.ScalarAssignedOverACompoundReplacesTheName for the panel.
//
// Every case here asserts the **listing** as well as the value, because the
// value alone cannot tell the two answers apart on the read a script usually
// does: `$a` is the first element under one answer and the whole of the name
// under the other, and both spell `x`. The listing is what says whether the
// array is still there, and it is the instrument every one of these bugs hid
// from.
func scalarOverCompoundSem(replaces Answer) Semantics {
	s := testSemantics()
	s.ScalarAssignedOverACompoundReplacesTheName = replaces
	return s
}

// The listing after each of the two answers, for an assignment standing as a
// command of its own — the one spelling that was already right, and the
// baseline every other spelling below is compared against.
func TestAScalarAssignedOverAnArray(t *testing.T) {
	for _, c := range []struct {
		replaces Answer
		want     string
	}{
		{Yes, `declare -- a="x"`},
		{No, `declare -a a=([0]="x" [1]="2" [2]="3")`},
	} {
		out, st := run(t, `a=(1 2 3); a=x; typeset -p a`, withSem(scalarOverCompoundSem(c.replaces)))
		if got := strings.TrimSpace(out); got != c.want || st != 0 {
			t.Errorf("replaces=%v: got %q status %d, want %q", c.replaces, got, st, c.want)
		}
	}
}

// The same question asked of a keyed table, which answers it the same way in
// every column — one field and not two.
func TestAScalarAssignedOverATable(t *testing.T) {
	for _, c := range []struct {
		replaces Answer
		want     string
	}{
		{Yes, `declare -- m="x"`},
		{No, `declare -A m=([0]="x" [k]="v" )`},
	} {
		sem := scalarOverCompoundSem(c.replaces)
		sem.DeclareOptions = "aAgilprux"
		out, st := run(t, `typeset -A m; m[k]=v; m=x; typeset -p m`, withSem(sem))
		if got := strings.TrimSpace(out); got != c.want || st != 0 {
			t.Errorf("replaces=%v: got %q status %d, want %q", c.replaces, got, st, c.want)
		}
	}
}

// The loop variable of a `for` is set by the same rule, which is the whole of
// why the axis is at the store: this read the array back on every pass under
// both answers, because the assignment *statement* was the only spelling that
// knew what a scalar does to an array.
//
// The body's read is what is asserted and not only the listing afterwards: a
// fix applied at the end of the loop would leave every pass wrong and the last
// line right.
func TestAForLoopOverANameHoldingAnArray(t *testing.T) {
	const src = `v=(a b c); for v in x y z; do printf "[%s]" "$v"; done; echo; typeset -p v`
	for _, c := range []struct {
		replaces Answer
		want     string
	}{
		{Yes, "[x][y][z]\ndeclare -- v=\"z\""},
		{No, "[x][y][z]\ndeclare -a v=([0]=\"z\" [1]=\"b\" [2]=\"c\")"},
	} {
		out, st := run(t, src, withSem(scalarOverCompoundSem(c.replaces)))
		if got := strings.TrimSpace(out); got != c.want || st != 0 {
			t.Errorf("replaces=%v: got %q status %d, want %q", c.replaces, got, st, c.want)
		}
	}
}

// A name given the array attribute and no elements answers the same way: the
// attribute is not a lock, and the value replaces it or lands in it exactly as
// it does for an array with something in it.
func TestAScalarAssignedOverADeclaredEmptyArray(t *testing.T) {
	for _, c := range []struct {
		replaces Answer
		want     string
	}{
		{Yes, `declare -- v="x"`},
		{No, `declare -a v=([0]="x")`},
	} {
		sem := scalarOverCompoundSem(c.replaces)
		sem.DeclareOptions = "aAgilprux"
		out, st := run(t, `typeset -a v; v=x; typeset -p v`, withSem(sem))
		if got := strings.TrimSpace(out); got != c.want || st != 0 {
			t.Errorf("replaces=%v: got %q status %d, want %q", c.replaces, got, st, c.want)
		}
	}
}

// The element the value lands on is the *base* and not the lowest subscript
// standing, which only a sparse array can show: the array grows a first
// element and what was at 5 does not move.
//
// The same place `a+=x` joins under its own axis's joining answer, which is
// why the two read one arrayBase between them.
func TestTheScalarLandsAtTheBaseOfASparseArray(t *testing.T) {
	sem := scalarOverCompoundSem(No)
	sem.DeclarationTakesASubscript = Yes
	out, st := run(t, `a=(1 2); a[5]=q; unset "a[0]"; unset "a[1]"; a=x; typeset -p a`, withSem(sem))
	const want = `declare -a a=([0]="x" [5]="q")`
	if got := strings.TrimSpace(out); got != want || st != 0 {
		t.Errorf("got %q status %d, want %q", got, st, want)
	}
}

// A prefix assignment is taken back whole. It is the one caller whose scalar
// store is undone, and the undo had only ever held the scalar table — so under
// the replacing answer the array was gone for good, and under the other the
// element it wrote was still there.
//
// `true` is a builtin, which is what makes the prefix transient at all.
func TestAPrefixAssignmentGivesBackTheCompound(t *testing.T) {
	for _, replaces := range []Answer{Yes, No} {
		out, st := run(t, `a=(p q); a=x true; typeset -p a`,
			withSem(scalarOverCompoundSem(replaces)))
		const want = `declare -a a=([0]="p" [1]="q")`
		if got := strings.TrimSpace(out); got != want || st != 0 {
			t.Errorf("replaces=%v: got %q status %d, want %q", replaces, got, st, want)
		}
	}
}

// And the store that keeps `$a` answering for an array `a` is not this
// question. Every element write refreshes it, so an answer of "replace" read
// there would take the array away the moment anything was written to it — and
// an answer of "write the first element" would send the refresh back through
// the store it came from.
//
// Both answers, because the exemption has to hold on both sides and the
// recursion is only reachable from one of them.
func TestAnElementWriteDoesNotDisturbTheArray(t *testing.T) {
	for _, c := range []struct {
		replaces Answer
		want     string
	}{
		{Yes, `declare -a a=([0]="1" [1]="z")`},
		{No, `declare -a a=([0]="1" [1]="z")`},
	} {
		sem := scalarOverCompoundSem(c.replaces)
		sem.DeclarationTakesASubscript = Yes
		out, st := run(t, `a=(1 2); a[1]=z; typeset -p a`, withSem(sem))
		if got := strings.TrimSpace(out); got != c.want || st != 0 {
			t.Errorf("replaces=%v: got %q status %d, want %q", c.replaces, got, st, c.want)
		}
	}
}

// An unanswered axis is refused rather than guessed, and the refusal leaves
// the array where it was: a store nobody could decide is a store that does not
// happen.
func TestAnUnansweredScalarOverACompoundIsRefused(t *testing.T) {
	sem := testSemantics()
	sem.ScalarAssignedOverACompoundReplacesTheName = Unspecified
	out, _ := run(t, `a=(1 2); a=x; typeset -p a`, withSem(sem))
	if !strings.Contains(out, "the shells disagree here and no dialect was chosen") {
		t.Errorf("got %q, want the unanswered-axis refusal", out)
	}
	if !strings.Contains(out, `declare -a a=([0]="1" [1]="2")`) {
		t.Errorf("got %q, want the array left as it was", out)
	}
}
