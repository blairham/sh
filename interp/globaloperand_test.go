// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// The two routes a `-g` declaration reaches that the axis was not asked at:
// an **array literal**, which is a command operand assigned after the builtin
// has returned, and a prefix in front of the declaration **itself**, which is
// the running command's own entry rather than an enclosing call's.
//
// Both are Semantics.DeclareGlobalReachesPastALocal again and neither is a
// second axis — the letter either names the shell's own cell, in which case
// the write goes under whatever is standing on the name, or it means "take no
// new local", in which case it writes what is visible.
//
// Named for the axis and never for a shell, as everything in this package is.

// globalLiteral is globalLetter with the two compound questions an array
// literal standing over a scalar — or a scalar prefix standing over an array —
// puts on its way through, so that these rows read the letter's answer rather
// than an unanswered axis beside it.
func globalLiteral(reaches, fresh Answer) func(*Semantics) {
	return func(s *Semantics) {
		globalLetter(reaches)(s)
		s.ScalarUnderAnArrayDeclaration = ScalarUnderACompoundDiscardsIt
		s.ScalarAssignedOverACompoundReplacesTheName = Yes
		// One row below writes a prefix over a name holding an array, which
		// is a second question about the same moment — see
		// Semantics.AssignmentPrefixMakesAFreshCell. It is swept rather than
		// chosen, because the letter's answer is the same under both: what
		// the `-g` write goes *under* is the prefix's entry, whether that
		// entry is the binding it displaced or a cell of its own.
		s.AssignmentPrefixMakesAFreshCell = fresh
	}
}

// The literal route, with each of the three things that can stand on the
// name: nothing, a local, and an enclosing call's prefix.
//
// The first row is the control and is what says the letter reaches this route
// at all — with nothing in the way both answers write the shell's own cell,
// so a difference in the other two is about the shadow and not about the
// operand.
func TestAGlobalArrayLiteralWritesUnderWhatStandsOnTheName(t *testing.T) {
	for _, tc := range []struct {
		name    string
		src     string
		reaches Answer
		want    string
	}{
		{
			name:    "nothing in the way",
			src:     "z=(9 9); g() { typeset -ga z=(1 2); }; g; echo \"[${z[@]}]\"",
			reaches: Yes, want: "[1 2]\n",
		},
		{
			name:    "nothing in the way, the other answer",
			src:     "z=(9 9); g() { typeset -ga z=(1 2); }; g; echo \"[${z[@]}]\"",
			reaches: No, want: "[1 2]\n",
		},
		{
			name: "a local of the same name",
			src: `y=(9 9); f() { typeset -a y=(5); typeset -ga y=(1 2); echo "1[${y[@]}]"; }
f; echo "2[${y[@]}]"`,
			reaches: Yes, want: "1[5]\n2[1 2]\n",
		},
		{
			name: "a local of the same name, the other answer",
			src: `y=(9 9); f() { typeset -a y=(5); typeset -ga y=(1 2); echo "1[${y[@]}]"; }
f; echo "2[${y[@]}]"`,
			reaches: No, want: "1[1 2]\n2[9 9]\n",
		},
		{
			name: "an enclosing call's assignment prefix",
			src: `w=(9 9); h() { typeset -ga w=(1 2); }
w=7 h; echo "[${w[@]}]"`,
			reaches: Yes, want: "[1 2]\n",
		},
		{
			name: "an enclosing call's assignment prefix, the other answer",
			src: `w=(9 9); h() { typeset -ga w=(1 2); }
w=7 h; echo "[${w[@]}]"`,
			reaches: No, want: "[9 9]\n",
		},
		{
			name:    "the declaration's own assignment prefix",
			src:     `q=7 typeset -ga q=(1 2); echo "[${q[@]-U}]"`,
			reaches: Yes, want: "[1 2]\n",
		},
		{
			name:    "the declaration's own assignment prefix, the other answer",
			src:     `q=7 typeset -ga q=(1 2); echo "[${q[@]-U}]"`,
			reaches: No, want: "[U]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, fresh := range []Answer{Yes, No} {
				out, errs, st := declRun(t, tc.src, globalLiteral(tc.reaches, fresh), Diagnostics{})
				if out != tc.want || st != 0 || errs != "" {
					t.Errorf("%v (fresh=%v) = %q (stderr %q, status %d), want %q",
						tc.reaches, fresh, out, errs, st, tc.want)
				}
			}
		})
	}
}

// And the same route with the parentheses **quoted**, which the machinery
// above cannot see.
//
// Runner.literalOperands is filled from the parser's `c.Assigns`, and
// `typeset -ga "y=(1 2)"` is one ordinary word — so the letter was read,
// recorded, and had nothing to act on. The value was re-read as a literal and
// then stored into whatever local was standing on the name, which is the one
// place a `-g` declaration must never write.
//
// The rows are the unquoted ones above with the quotes put on, deliberately,
// so that a reading which is right there and wrong here cannot pass: the two
// spellings are one request in the column that re-reads them, and a test that
// only asked the quoted form could not say that.
//
// The last two rows are the shapes the quoted form has and the written one
// does not — a table, and an append — because the store branches on both and
// each branch chooses its cell separately (#4163).
func TestAGlobalQuotedArrayLiteralWritesUnderWhatStandsOnTheName(t *testing.T) {
	rereads := func(reaches, fresh Answer) func(*Semantics) {
		return func(s *Semantics) {
			globalLiteral(reaches, fresh)(s)
			s.DeclarationRereadsAParenthesizedValue = Yes
			// The append row below writes `name+=value` on a declaration
			// line, which is a question about whether the operand is an
			// append at all — swept out of the way rather than chosen,
			// because the letter's answer is the same either way and a row
			// that stopped on an unanswered axis would be measuring that
			// axis instead. See Semantics.DeclarationTakesAnAppendOperand.
			s.DeclarationTakesAnAppendOperand = Yes
		}
	}
	for _, tc := range []struct {
		name    string
		src     string
		reaches Answer
		want    string
	}{
		{
			name:    "nothing in the way",
			src:     `z=(9 9); g() { typeset -ga z="(1 2)"; }; g; echo "[${z[@]}]"`,
			reaches: Yes, want: "[1 2]\n",
		},
		{
			name:    "nothing in the way, the other answer",
			src:     `z=(9 9); g() { typeset -ga z="(1 2)"; }; g; echo "[${z[@]}]"`,
			reaches: No, want: "[1 2]\n",
		},
		{
			name: "a local of the same name",
			src: `y=(9 9); f() { typeset -a y=(5); typeset -ga y="(1 2)"; echo "1[${y[@]}]"; }
f; echo "2[${y[@]}]"`,
			reaches: Yes, want: "1[5]\n2[1 2]\n",
		},
		{
			name: "a local of the same name, the other answer",
			src: `y=(9 9); f() { typeset -a y=(5); typeset -ga y="(1 2)"; echo "1[${y[@]}]"; }
f; echo "2[${y[@]}]"`,
			reaches: No, want: "1[1 2]\n2[9 9]\n",
		},
		{
			name: "the name arrives from an expansion",
			src: `y=(9 9); f() { typeset -a y=(5); n=y; typeset -ga "$n=(1 2)"; echo "1[${y[@]}]"; }
f; echo "2[${y[@]}]"`,
			reaches: Yes, want: "1[5]\n2[1 2]\n",
		},
		{
			name: "a table standing under a local",
			src: `typeset -A m=([z]=9); f() { typeset -A m=([z]=5); typeset -gA m="([k]=v)"; echo "1[${m[k]-U}]"; }
f; echo "2[${m[k]-U}]"`,
			reaches: Yes, want: "1[U]\n2[v]\n",
		},
		{
			name: "an append standing under a local",
			src: `y=(9); f() { typeset -a y=(5); typeset -ga y+="(1)"; echo "1[${y[@]}]"; }
f; echo "2[${y[@]}]"`,
			reaches: Yes, want: "1[5]\n2[9 1]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, fresh := range []Answer{Yes, No} {
				out, errs, st := declRun(t, tc.src, rereads(tc.reaches, fresh), Diagnostics{})
				if out != tc.want || st != 0 || errs != "" {
					t.Errorf("%v (fresh=%v) = %q (stderr %q, status %d), want %q",
						tc.reaches, fresh, out, errs, st, tc.want)
				}
			}
		})
	}
}

// The scalar spelling under the declaration's **own** prefix, which is the
// third stack and the innermost: a `-g` write goes under the entry this
// command's prefix made, so the prefix's value goes away as it does after any
// other command and the declaration's outlives it.
//
// The first row is the control that keeps this about the letter: without it
// the prefix's value goes away in both columns, and nothing is left.
func TestAGlobalDeclarationWritesUnderItsOwnAssignmentPrefix(t *testing.T) {
	for _, tc := range []struct {
		name    string
		src     string
		reaches Answer
		want    string
	}{
		{
			name:    "no letter: the prefix leaves with the command",
			src:     `w=7 typeset w=3; echo "[${w-U}]"`,
			reaches: Yes, want: "[U]\n",
		},
		{
			name:    "the letter writes underneath",
			src:     `x=7 typeset -g x=3; echo "[${x-U}]"`,
			reaches: Yes, want: "[3]\n",
		},
		{
			name:    "the other answer writes the binding that is visible",
			src:     `x=7 typeset -g x=3; echo "[${x-U}]"`,
			reaches: No, want: "[U]\n",
		},
		{
			name:    "over a cell that was already there",
			src:     `v=1; v=7 typeset -g v=3; echo "[${v-U}]"`,
			reaches: Yes, want: "[3]\n",
		},
		{
			name:    "over a cell that was already there, the other answer",
			src:     `v=1; v=7 typeset -g v=3; echo "[${v-U}]"`,
			reaches: No, want: "[1]\n",
		},
		{
			name:    "a declaration with no value writes no value",
			src:     `y=7 typeset -g y; echo "[${y-U}]"`,
			reaches: Yes, want: "[U]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := declRun(t, tc.src, globalLetter(tc.reaches), Diagnostics{})
			if out != tc.want || st != 0 || errs != "" {
				t.Errorf("%v = %q (stderr %q, status %d), want %q",
					tc.reaches, out, errs, st, tc.want)
			}
		})
	}
}

// The promotion question is **not put** for a name the letter is writing
// underneath, which is the one place this change reaches an answer that was
// already right for every other declaration.
//
// One column keeps the value its own prefix set where the declaration in
// front of it names the export or readonly attribute over that same name —
// DeclarationPromotesThePrefixEntry. A `-g` declaration is not writing that
// binding, so there is nothing for it to keep, and what is left behind is
// what the declaration put on the cell underneath.
func TestTheGlobalLetterTakesThePromotionQuestionAway(t *testing.T) {
	promotes := func(reaches Answer) func(*Semantics) {
		return func(s *Semantics) {
			globalLetter(reaches)(s)
			s.DeclarationPromotesThePrefixEntry = Yes
		}
	}
	for _, tc := range []struct {
		name    string
		src     string
		reaches Answer
		want    string
	}{
		{
			// The control: no letter, and the prefix's value is kept.
			name:    "the attribute keeps the prefix's value",
			src:     `a=7 typeset -r a; echo "[${a-U}]"`,
			reaches: Yes, want: "[7]\n",
		},
		{
			name:    "the letter writes the cell underneath instead",
			src:     `b=7 typeset -gr b=3; echo "[${b-U}]"`,
			reaches: Yes, want: "[3]\n",
		},
		{
			// And with no value of its own there is nothing to leave: the
			// attribute lands on the cell underneath and the prefix's value
			// goes away like any other.
			name:    "the letter with no value keeps no value",
			src:     `d=7 typeset -gr d; echo "[${d-U}]"`,
			reaches: Yes, want: "[U]\n",
		},
		// The other answer is not paired with this one in the panel — the
		// column that keeps a prefix entry is the same column that writes
		// underneath — so there is no row for it here. What the combination
		// does is write the visible binding and then keep it, which is each
		// half doing what it says and is nobody's measured behavior.
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := declRun(t, tc.src, promotes(tc.reaches), Diagnostics{})
			if out != tc.want || st != 0 || errs != "" {
				t.Errorf("%v = %q (stderr %q, status %d), want %q",
					tc.reaches, out, errs, st, tc.want)
			}
		})
	}
}
