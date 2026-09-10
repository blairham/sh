// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// What a declaration finds in the cell a scope was just taken for: nothing,
// including the array or the table the outer name held. See
// interp/freshcell.go for where the rows were measured.
//
// Every test here keeps the caller's name in its assertion, because the
// restore is the control: a fix that took the elements away for good rather
// than out of the fresh cell's view would pass the first half of each of
// these and fail the second.
//
// The two readings of a valueless declaration are asked side by side wherever
// they can both answer. One of them took the compound away already, by
// hiding the whole name, and that is exactly what kept this bug to one
// dialect and to one of the four shapes below.

// withFreshCells is withArrayLetters with an answer for what a *scalar* store
// does to a name already holding a compound. The merging answer is the one
// the rows with a value use, and using it is the point: if the fresh cell
// were really that axis in disguise, those rows would merge.
func withFreshCells(empty, replaces Answer) func(*Semantics) {
	letters := withArrayLetters(empty)
	return func(s *Semantics) {
		letters(s)
		s.ScalarAssignedOverACompoundReplacesTheName = replaces
	}
}

// The reported shape: an array letter, no value, over a caller's array. The
// listing is the instrument rather than `${#a[@]}` alone, because it is the
// only read that says the name is still an array while holding nothing.
func TestAValuelessArrayDeclarationDropsTheOuterElements(t *testing.T) {
	src := `a=(x y)
f() { local -a a; echo "in n=${#a[@]} [${a[*]}]"; typeset -p a; }
f
echo "after n=${#a[@]} [${a[*]}]"`
	out, errs, st := declRun(t, src, withFreshCells(Yes, No), Diagnostics{})
	const want = "in n=0 []\ndeclare -a a=()\nafter n=2 [x y]\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("= %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// And without the letter, which is the row a fix aimed at `-a` alone does not
// reach: the name is an ordinary empty scalar inside the function and the
// caller's elements are not in view through it either.
//
// `${#a[@]}` is deliberately not the instrument here. Under the reading that
// sets a declared name empty the name is a scalar holding the empty string,
// and one of those counts as a single element — so this row answers 1 when it
// is right and 2 when it is wrong, and 1 is also what a scalar that had never
// held anything answers. The listing and a subscript past the first are what
// tell the fresh cell from the caller's.
func TestAValuelessDeclarationWithoutTheArrayLetterDropsThemToo(t *testing.T) {
	src := `a=(x y)
f() { local a; echo "in [${a[*]}] [${a[1]-NONE}]"; typeset -p a; }
f
echo "after n=${#a[@]} [${a[*]}]"`
	out, errs, st := declRun(t, src, withFreshCells(Yes, No), Diagnostics{})
	const want = "in [] [NONE]\ndeclare -- a=\"\"\nafter n=2 [x y]\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("= %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// Both readings of a valueless declaration agree, which is what makes this
// the core's rule rather than a second answer beside
// DeclaredNameWithoutValueIsEmpty. The listing is left out here because the
// two readings word it differently — one has a name to list and the other has
// none — and what they agree about is that the caller's elements are gone.
//
// So is `${#a[@]}`, and for the reason the row above gives: the two readings
// disagree about it for the spellings without a letter, where one leaves an
// empty scalar and the other leaves nothing at all. What is left is what the
// bug was about — whether the caller's elements answer a read of the name
// this function declared. The first element is left out for the same reason,
// since one reading answers it with the empty scalar it made and the other
// with nothing; the *second* is the caller's alone under both.
func TestNeitherReadingOfAValuelessDeclarationKeepsTheOuterElements(t *testing.T) {
	for _, c := range []struct {
		name  string
		empty Answer
	}{
		{"set empty", Yes},
		{"hidden", No},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, decl := range []string{"local -a a", "local a", "typeset -a a", "typeset a"} {
				t.Run(decl, func(t *testing.T) {
					src := `a=(x y)
f() { ` + decl + `; echo "in [${a[*]}] [${a[1]-NONE}]"; }
f
echo "after n=${#a[@]} [${a[*]}]"`
					out, errs, st := declRun(t, src, withFreshCells(c.empty, No), Diagnostics{})
					const want = "in [] [NONE]\nafter n=2 [x y]\n"
					if out != want || errs != "" || st != 0 {
						t.Errorf("%s = %q (stderr %q, status %d), want %q", decl, out, errs, st, want)
					}
				})
			}
		})
	}
}

// The keyed table is the other compound and lives in a table of its own
// again, so it is its own row rather than a corollary: a declaration that
// dropped only the elements would leave a caller's table in the fresh cell.
func TestAValuelessDeclarationDropsTheOuterTable(t *testing.T) {
	src := `typeset -A m; m[k]=v; m[j]=w
f() { local -A m; echo "in n=${#m[@]} [${m[k]-NONE}]"; typeset -p m; }
f
echo "after n=${#m[@]} [${m[k]}]"`
	out, errs, st := declRun(t, src, withFreshCells(Yes, No), Diagnostics{})
	const want = "in n=0 [NONE]\ndeclare -A m\nafter n=2 [v]\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("= %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// A declaration that *assigns* into a fresh cell, under the answer that
// merges a scalar store into a standing compound. The merge is what the
// caller's array would get and is what the top-level control below still
// gets; here there is nothing to merge with, so the name is the scalar the
// declaration wrote.
func TestADeclarationWithAValueIntoAFreshCellIsNotMergedWithTheOuterArray(t *testing.T) {
	src := `a=(x y)
f() { local a=q; typeset -p a; }
f
echo "after [${a[*]}]"`
	out, errs, st := declRun(t, src, withFreshCells(Yes, No), Diagnostics{})
	const want = "declare -- a=\"q\"\nafter [x y]\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("= %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// The control that makes the row above about the cell and not about the
// axis: the same declaration where no scope was taken merges, because there
// the compound in front of it is the one the name is really holding.
func TestADeclarationWithAValueAndNoScopeIsStillMergedWithTheArray(t *testing.T) {
	src := `a=(x y); typeset a=q; typeset -p a`
	out, errs, st := declRun(t, src, withFreshCells(Yes, No), Diagnostics{})
	const want = "declare -a a=([0]=\"q\" [1]=\"y\")\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("= %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// And the second control, from inside a function: the letter that declines
// the scope declines the fresh cell with it, so a global declaration merges
// where the local one beside it does not.
func TestAGlobalDeclarationInsideAFunctionKeepsTheOuterArray(t *testing.T) {
	src := `a=(x y)
f() { typeset -g a=q; typeset -p a; }
f
echo "after [${a[*]}]"`
	out, errs, st := declRun(t, src, withFreshCells(Yes, No), Diagnostics{})
	const want = "declare -a a=([0]=\"q\" [1]=\"y\")\nafter [q y]\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("= %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// The third control, and the one a drop that asked the *scope* rather than
// the shadow would fail: a second declaration of a name this scope already
// declared is writing over a cell of its own, and the compound in front of it
// is the one the first declaration made.
func TestASecondDeclarationInTheSameScopeKeepsWhatTheFirstDeclared(t *testing.T) {
	src := `a=(x y)
f() { local -a a=(q w); local -a a; echo "in n=${#a[@]} [${a[*]}]"; }
f`
	out, errs, st := declRun(t, src, withFreshCells(Yes, No), Diagnostics{})
	const want = "in n=2 [q w]\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("= %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// The subscripted operand is the third declaration loop, and it reaches the
// drop through the same shared step the other two do. Written into each loop
// instead, this is the row that would have been missed — as the compound mark
// itself was, in #1535.
func TestASubscriptedDeclarationIntoAFreshCellWritesTheOnlyElement(t *testing.T) {
	src := `a=(x y z)
f() { local a[1]=q; echo "in n=${#a[@]} [${a[*]}] [${a[0]-NONE}]"; }
f
echo "after [${a[*]}]"`
	sparse := func(s *Semantics) {
		withFreshCells(Yes, No)(s)
		// Sparse, so the element written is the only one there is. Under the
		// answer that fills the gap the fresh cell holds two elements and one
		// of them is empty, which is a true row about a different axis and
		// says nothing about whose elements these are.
		s.ArraysAreSparse = Yes
	}
	out, errs, st := declRun(t, src, sparse, Diagnostics{})
	const want = "in n=1 [q] [NONE]\nafter [x y z]\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("= %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// A nested call, because the rule is about the innermost scope and not about
// the global cell: the caller here is itself a function that assigned the
// array, and the callee's declaration must not see those elements either.
func TestAFreshCellInANestedCallDropsTheCallersElements(t *testing.T) {
	src := `a=(x y)
inner() { local -a a; echo "in n=${#a[@]} [${a[*]}]"; }
outer() { a=(p q r); inner; echo "outer n=${#a[@]} [${a[*]}]"; }
outer
echo "after n=${#a[@]} [${a[*]}]"`
	out, errs, st := declRun(t, src, withFreshCells(Yes, No), Diagnostics{})
	const want = "in n=0 []\nouter n=3 [p q r]\nafter n=3 [p q r]\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("= %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// The shape the bug was found in: a declaration of several names at once,
// where the first is the one the caller filled and the function appends to
// it. The append is what turns a stale cell into wrong output rather than
// merely into a wrong length.
func TestAFreshCellAppendedToHoldsOnlyWhatTheFunctionAdded(t *testing.T) {
	src := `files=(stale1 stale2)
audit() { local -a files extra; files+=(real1); echo "in n=${#files[@]} [${files[*]}] extra=${#extra[@]}"; }
audit
echo "after n=${#files[@]} [${files[*]}]"`
	out, errs, st := declRun(t, src, withFreshCells(Yes, No), Diagnostics{})
	const want = "in n=1 [real1] extra=0\nafter n=2 [stale1 stale2]\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("= %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}
