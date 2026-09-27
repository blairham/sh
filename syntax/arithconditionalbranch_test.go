// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// The level each branch of `c ? t : e` is read at, one branch at a time.
//
// The two fields are named here and the shells that set them are not. Each
// branch has a level of its own because the panel does not hold them at the
// same one, and the rows below move one field while holding the other, which
// is what says the two are separable rather than one setting spelled twice.
//
// It is a level and not a ban, which the parenthesized rows carry: the same
// store in the same position is taken at every level, so what a level moves is
// where a *bare* assignment or a comma may begin a branch.
func TestEachConditionalBranchIsReadAtItsOwnLevel(t *testing.T) {
	t.Parallel()
	at := func(then, els syntax.ArithConditionalBranchLevel) syntax.Dialect {
		d := syntax.Core()
		d.ArithConditionalThenLevel, d.ArithConditionalElseLevel = then, els
		return d
	}
	const (
		seq  = syntax.ArithConditionalBranchSequence
		asgn = syntax.ArithConditionalBranchAssignment
		cond = syntax.ArithConditionalBranchConditional
	)
	for _, tc := range []struct {
		name string
		src  string
		// refused at each of the three levels of the branch the row is
		// about, loosest first.
		then    bool // the row varies the then level; else varies the else
		refused [3]bool
	}{
		{"a bare assignment in the then", `1 ? x = 2 : 3`, true, [3]bool{false, false, true}},
		{"a bare compound assignment in the then", `0 ? x += 2 : 3`, true, [3]bool{false, false, true}},
		{"a comma in the then", `1 ? 2 , 3 : 4`, true, [3]bool{false, true, true}},
		{"a parenthesized assignment in the then", `1 ? (x = 2) : 3`, true, [3]bool{false, false, false}},
		{"an increment in the then", `1 ? x++ : 3`, true, [3]bool{false, false, false}},
		{"a conditional inside the then", `1 ? 0 ? 5 : 6 : 3`, true, [3]bool{false, false, false}},

		{"a bare assignment in the else", `1 ? 2 : x = 3`, false, [3]bool{false, false, true}},
		{"a comma in the else", `1 ? 2 : 3 , 4`, false, [3]bool{false, false, false}},
		{"a parenthesized assignment in the else", `1 ? 2 : (x = 3)`, false, [3]bool{false, false, false}},
		{"a conditional inside the else", `0 ? 1 : 0 ? 2 : 3`, false, [3]bool{false, false, false}},

		{"an ordinary conditional", `1 ? 2 : 3`, true, [3]bool{false, false, false}},
		{"an assignment around the whole of it", `x = 1 ? 2 : 3`, false, [3]bool{false, false, false}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for i, level := range [3]syntax.ArithConditionalBranchLevel{seq, asgn, cond} {
				// The branch the row is not about is held at its loosest, so
				// nothing it could refuse is what the row is reading.
				d := at(level, seq)
				if !tc.then {
					d = at(seq, level)
				}
				err := arithErr(tc.src, d)
				if tc.refused[i] && err == nil {
					t.Errorf("read %q with the branch at %v: no error, want one", tc.src, level)
				}
				if !tc.refused[i] && err != nil {
					t.Errorf("read %q with the branch at %v: %v, want no error", tc.src, level, err)
				}
			}
		})
	}
}

// The comma is above a branch read at the sequence level and below one read at
// either of the other two, so `1 ? 2 , 3 : 4` is one expression in the then
// branch under the loosest reading and a refusal under the other two — while
// `1 ? 2 : 3 , 4` belongs to the expression *around* the conditional however
// the else branch is read, and so is taken at all three.
//
// That asymmetry is the whole reason a sequence level is worth having: without
// it the first row is unreachable and the second says nothing.
func TestTheSequenceLevelIsReachableOnlyInTheThenBranch(t *testing.T) {
	t.Parallel()
	d := syntax.Core()
	if !d.ArithComma {
		t.Fatal("the core has no sequence operator, so neither row measures a level")
	}
	d.ArithConditionalThenLevel = syntax.ArithConditionalBranchSequence
	d.ArithConditionalElseLevel = syntax.ArithConditionalBranchConditional
	if err := arithErr(`1 ? 2 , 3 : 4`, d); err != nil {
		t.Errorf("a comma in a sequence-level then branch: %v, want no error", err)
	}
	d.ArithConditionalThenLevel = syntax.ArithConditionalBranchAssignment
	if err := arithErr(`1 ? 2 , 3 : 4`, d); err == nil {
		t.Error("a comma in an assignment-level then branch: no error, want one")
	}
	if err := arithErr(`1 ? 2 : 3 , 4`, d); err != nil {
		t.Errorf("a comma after the conditional: %v, want no error", err)
	}
}

// A dialect with no sequence operator cannot tell the two loosest levels
// apart, which is what the zero value means and is why dash holds it.
func TestWithoutTheOperatorTheTwoLoosestLevelsAgree(t *testing.T) {
	t.Parallel()
	d := syntax.POSIX()
	if d.ArithComma {
		t.Fatal("this preset has the sequence operator, so the two levels are distinguishable")
	}
	for _, src := range []string{`1 ? x = 2 : 3`, `0 ? x += 2 : 3`, `1 ? 2 : 3`} {
		d.ArithConditionalThenLevel = syntax.ArithConditionalBranchSequence
		seq := arithErr(src, d)
		d.ArithConditionalThenLevel = syntax.ArithConditionalBranchAssignment
		asgn := arithErr(src, d)
		if (seq == nil) != (asgn == nil) {
			t.Errorf("%q: sequence gave %v and assignment gave %v, want the same", src, seq, asgn)
		}
	}
}
