// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// conditionQualifying is the grammar a qualifier list inside a condition
// needs, named by the constructs rather than by a shell.
func conditionQualifying(d *syntax.Dialect) {
	qualifying(d)
	d.DoubleBracket = true
	d.ParamTildeFlag = true
}

// conditionDir is two files, one of which the rows below ask for by a name
// nothing has.
func conditionDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// runCondition runs src in dir with the extended pattern spelling on, which
// is what makes `(#q…)` a qualifier group anywhere.
func runCondition(t *testing.T, dir, src string) (string, int) {
	t.Helper()
	d := syntax.Core()
	conditionQualifying(&d)
	return runGrammar(t, src, conditionQualifying, func(r *Runner) {
		r.Dialect = &d
		r.Dir = dir
		sem := *r.Semantics
		sem.GlobNoMatchIsError = Yes
		// The reading that leaves a value's group syntax live, which is every
		// column but one and is not what these rows are about: the qualifier
		// group has to be *written* to be read, and that is a rule of the
		// word rather than of the value — see
		// Semantics.ExpansionResultSuppliesGroupSyntax.
		sem.ExpansionResultSuppliesGroupSyntax = Yes
		r.Semantics = &sem
		r.SetMatchOption(ExtendedPatternOperators, true)
	})
}

// A condition operand ending in a glob qualifier group is matched against the
// filesystem, which is the one thing in a condition that is.
//
// The vendor manual states it as a rule and every row is a measurement on
// zsh 5.9.2, in exactly the directory conditionDir builds. See
// interp/condqualifier.go for the whole table and for what it is load-bearing
// for.
func TestAConditionOperandEndingInAQualifierGroupIsMatched(t *testing.T) {
	dir := conditionDir(t)
	for _, tc := range []struct{ name, src, want string }{
		// The pair that says the group is doing the work: the same word with
		// and without it, over a name nothing has.
		{"a name nothing has comes to nothing", `[[ -n zz(#qN) ]] && printf yes || printf no`, "no"},
		{"and without the group it is text", `[[ -n zz ]] && printf yes || printf no`, "yes"},
		{"a name something has stays", `[[ -n a.txt(#qN) ]] && printf yes || printf no`, "yes"},
		// A file test's operand, and the left side of a comparison: both are
		// read through the same step, which is what the manual's "within
		// conditions" means.
		{"a file test's operand is matched too", `[[ -e zz(#qN) ]] && printf yes || printf no`, "no"},
		{"and the same operand that exists is not", `[[ -e a.txt(#qN) ]] && printf yes || printf no`, "yes"},
		{"the left side of a comparison is matched", `[[ a.txt(#qN) == a.txt ]] && printf yes || printf no`, "yes"},
		// Two matches are one operand with a space in it rather than two
		// operands, which is the row a reading that took the first match
		// would get wrong.
		{"two matches join with a space", `[[ *.txt(#qN) == "a.txt b.txt" ]] && printf yes || printf no`, "yes"},
		{"so the first match alone is not the answer", `[[ *.txt(#qN) == a.txt ]] && printf yes || printf no`, "no"},

		// The group has to be written, at the end, and unquoted. Every row
		// here is the word staying the text it was.
		{"a quoted word is text", `[[ -n "zz(#qN)" ]] && printf yes || printf no`, "yes"},
		{"and a quoted group is too", `[[ -n zz"(#qN)" ]] && printf yes || printf no`, "yes"},
		{"a group held in a parameter is a value", `V='zz(#qN)'; [[ -n $V ]] && printf yes || printf no`, "yes"},
		// The sharpest of them: the tilde flag makes a value's pattern
		// characters live everywhere else, and this is read off the word as
		// written rather than off what the expansion came to.
		{"even with the tilde flag on it", `V='zz(#qN)'; [[ -n ${~V} ]] && printf yes || printf no`, "yes"},
		{"a group not at the end is text", `[[ -n zz(#qN)x ]] && printf yes || printf no`, "yes"},
		// A prefix the word expanded is still matched, so it is the group's
		// spelling that is read and not the whole word's.
		{"a parameter in front of the group is matched", `P=zz; [[ -n $P(#qN) ]] && printf yes || printf no`, "no"},
		{"and one naming a file that is there", `P=a.txt; [[ -n $P(#qN) ]] && printf yes || printf no`, "yes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runCondition(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// Without the `N`, a miss inside a condition is the same miss it is anywhere
// else — which is what says the word really went to the filesystem rather
// than being emptied by a rule of its own.
func TestAConditionOperandThatMatchesNothingIsTheDialectsMiss(t *testing.T) {
	dir := conditionDir(t)
	out, st := runCondition(t, dir, `[[ -n zz(#q) ]] && printf yes || printf no`)
	if st == 0 || !containsSub(out, "no matches found") {
		t.Errorf("got %q (status %d), want the pattern reported as matching nothing", out, st)
	}
}
