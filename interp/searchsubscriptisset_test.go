// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// searchIsSetGrammar is `[[ -v ]]` over a subscript that carries a flag
// group: the operator, the subscript, and the group.
func searchIsSetGrammar(d *syntax.Dialect) {
	d.ArraySubscript = true
	d.ArraySubscriptFlags = true
	d.ParameterIsSetTest = true
	d.ArrayLiteral = true
	d.ParamExpansionFlags = true
	d.ParamElementSelection = true
}

// runSearchIsSet runs src with that grammar and the measured shell's array
// base. base zero is the one axis a row below varies deliberately, so it is a
// parameter rather than a constant here.
func runSearchIsSet(t *testing.T, src string, baseZero Answer) (string, int) {
	t.Helper()
	return runGrammar(t, src, searchIsSetGrammar, func(r *Runner) {
		d := syntax.Core()
		searchIsSetGrammar(&d)
		r.Dialect = &d
		sem := *r.Semantics
		sem.ArrayBaseIsZero = baseZero
		sem.GlobExpansionResults = No
		sem.SplitParamExpansion = No
		sem.GlobNoMatchIsError = Yes
		sem.SubscriptIsAQuotingContext = No
		// The measured shell's answer, needed only by the `[@]` row below,
		// which is here as the *bound* on the scalar rule rather than as an
		// assertion about this axis — wholearraysubscriptisset_test.go is
		// what grades the axis itself.
		sem.ConditionWholeArraySubscript = ConditionWholeArraySubscriptNamesAnElement
		r.Semantics = &sem
	})
}

// A search subscript in `[[ -v ]]` is the one operand shape where the
// operator does **not** answer what the conditional expansion answers.
//
// Measured 2026-09-29 on zsh 5.9.2 over a script file. The rows that carry
// the rule are the ones where `${x+SET}` says set and the operator says
// unset — `arr[(i)d]`, `arr[(i)x]`, `m[(i)nope]` and `m[(r)zz]` below — so a
// pair is asserted for each, and a return to the expansion's answer turns
// each pair into a disagreement. See Runner.searchSubscriptIsSet.
func TestASearchSubscriptIsSetDoesNotFollowTheExpansion(t *testing.T) {
	const arr = "arr=(a b c d)\n"
	for _, tc := range []struct{ name, sub, want string }{
		// `(i)` names an index, and that index is read as a **zero-based**
		// offset: an array of four has elements at 0, 1, 2 and 3 under this
		// reading, so the index `4` its own last element is found at is out
		// of range and the search that matched reports unset.
		{"a match near the front", "(i)a", "SET"},
		{"a match in the middle", "(i)c", "SET"},
		{"the last element's index is one too far", "(i)d", "UNSET"},
		{"a forward miss is further still", "(i)x", "UNSET"},
		// `(I)` searches backwards and misses to the index below the array,
		// which under the same zero-based reading is a live offset.
		{"a backward match", "(I)b", "SET"},
		{"the backward miss lands in range", "(I)x", "SET"},
		{"a backward match on the last element", "(I)d", "UNSET"},
		// `(r)` and `(R)` name a *value*, and there the question is the
		// ordinary one: whether the position the search named holds an
		// element.
		{"a value search that matched", "(r)a", "SET"},
		{"a value search that missed", "(r)zz", "UNSET"},
		{"a backward value search that matched", "(R)d", "SET"},
		{"a backward value search that missed", "(R)zz", "UNSET"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := arr + "[[ -v 'arr[" + tc.sub + "]' ]] && echo SET || echo UNSET\n"
			got, st := runSearchIsSet(t, src, No)
			if got != tc.want+"\n" || st != 0 {
				t.Errorf("`-v arr[%s]`: got %q/%d, want %q/0", tc.sub, got, st, tc.want)
			}
		})
	}
}

// The zero-based reading is what makes the rule the *index*'s and not the
// subscript's, and the base is the only thing that can say so: under a base
// of one the operator's set indices are 0 through 3 where an ordinary
// subscript's are 1 through 4, and under a base of zero the two coincide.
//
// So this asserts the pair that parts. Measured on zsh 5.9.2 with and
// without `setopt KSH_ARRAYS`: `arr[(i)d]` is index 4 and unset in the first
// and index 3 and set in the second, on the same four elements — while
// `arr[4]` written out is set in the first. A reading that used the base
// would answer set in both, and every other row in this file would still
// have agreed.
func TestTheSearchIndexIsReadAsAZeroBasedOffsetInEitherBase(t *testing.T) {
	const arr = "arr=(a b c d)\n"
	for _, tc := range []struct {
		name     string
		baseZero Answer
		sub      string
		want     string
	}{
		{"base one, the last element's search index", No, "(i)d", "UNSET"},
		{"base zero, the last element's search index", Yes, "(i)d", "SET"},
		{"base zero, one past the last", Yes, "(i)x", "UNSET"},
		// The written subscript keeps the base, which is the row that says
		// the two readings are different questions rather than one rule.
		{"base one, the last element written out", No, "4", "SET"},
		{"base zero, the last element written out", Yes, "3", "SET"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := arr + "[[ -v 'arr[" + tc.sub + "]' ]] && echo SET || echo UNSET\n"
			got, st := runSearchIsSet(t, src, tc.baseZero)
			if got != tc.want+"\n" || st != 0 {
				t.Errorf("`-v arr[%s]` base zero=%v: got %q/%d, want %q/0",
					tc.sub, tc.baseZero, got, st, tc.want)
			}
		})
	}
}

// A table is searched under every letter and the question is whether the
// search **matched** — not whether it substituted anything, which is a
// different question and agrees on all but one row.
//
// That row is `m[(r)]`, the empty pattern against a key whose value is
// empty: it matches, and the operator says set, while the text it
// substitutes is the empty string. Measured 2026-09-29 on zsh 5.9.2.
func TestATableSearchIsSetWhenTheSearchMatched(t *testing.T) {
	const m = "typeset -A m=(key val num 4 blank '')\n"
	for _, tc := range []struct{ name, sub, want string }{
		{"a key search that matched", "(i)key", "SET"},
		{"a key search that missed", "(i)nope", "UNSET"},
		{"every key, matched", "(I)*", "SET"},
		{"every key, missed", "(I)nope", "UNSET"},
		{"a value search that matched", "(r)val", "SET"},
		{"a value search that missed", "(r)zz", "UNSET"},
		// The row the "matched" reading is set from: an empty result from a
		// search that found something.
		{"a value search matching an empty value", "(r)", "SET"},
		{"a backward value search matching an empty value", "(R)", "SET"},
		// `(k)` and `(K)` look a key up rather than searching, so they keep
		// the plain key question — and the letter has to be read before the
		// mapping that folds `k` onto `r`, or the lookup searches the
		// table's *values* for the key's own text and finds nothing.
		{"a key lookup", "(k)key", "SET"},
		{"a key lookup that finds nothing", "(k)nope", "UNSET"},
		{"the other key lookup", "(K)key", "SET"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := m + "[[ -v 'm[" + tc.sub + "]' ]] && echo SET || echo UNSET\n"
			got, st := runSearchIsSet(t, src, No)
			if got != tc.want+"\n" || st != 0 {
				t.Errorf("`-v m[%s]`: got %q/%d, want %q/0", tc.sub, got, st, tc.want)
			}
		})
	}
}

// A scalar has no elements to name, so a subscript that names one finds
// nothing however well it indexes the characters — which is a different
// answer from the expansion's, where `${str[3]}` is a character.
//
// Measured 2026-09-29 on zsh 5.9.2 with `str='string'` and `typeset -i
// num=7`. The `[@]` row is the exception and is not this rule: that
// subscript names the parameter rather than an element, and
// wholeArraySubscriptIsSet already carries it. See
// Runner.scalarHasNoElements.
func TestAScalarHasNoElementsToNameInIsSet(t *testing.T) {
	const decl = "str=string\nempty=\narr=(a b c d)\ntypeset -A m=(k v)\n"
	for _, tc := range []struct{ name, operand, want string }{
		{"a character by index", "str[1]", "UNSET"},
		{"a character further in", "str[3]", "UNSET"},
		{"a range of characters", "str[1,3]", "UNSET"},
		{"a character from the end", "str[-1]", "UNSET"},
		{"a search over the characters", "str[(r)tr]", "UNSET"},
		{"an empty scalar", "empty[1]", "UNSET"},
		// The bounding rows: a name that *does* have elements keeps its
		// answer, so the rule is keyed on what the name holds and not on
		// what the subscript looks like.
		{"an array element", "arr[1]", "SET"},
		{"a table key", "m[k]", "SET"},
		// The whole-array subscript is the parameter's own set-ness, which
		// a scalar answers for itself.
		{"the whole-array subscript on a scalar", "str[@]", "SET"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := decl + "[[ -v '" + tc.operand + "' ]] && echo SET || echo UNSET\n"
			got, st := runSearchIsSet(t, src, No)
			if got != tc.want+"\n" || st != 0 {
				t.Errorf("`-v %s`: got %q/%d, want %q/0", tc.operand, got, st, tc.want)
			}
		})
	}
}

// The two spellings of the operator have to reach the same lookup, which is
// what says the search reading lives under both rather than in the
// condition's own route. Measured: all three of `[[ -v ]]`, `test -v` and
// `[ -v ]` agree on every row above in zsh 5.9.2.
func TestTheTestBuiltinAgreesAboutASearchSubscript(t *testing.T) {
	const arr = "arr=(a b c d)\n"
	for _, sub := range []string{"(i)a", "(i)d", "(i)x", "(I)x", "(r)zz"} {
		cond := arr + "[[ -v 'arr[" + sub + "]' ]] && echo SET || echo UNSET\n"
		builtin := arr + "test -v 'arr[" + sub + "]' && echo SET || echo UNSET\n"
		a, _ := runSearchIsSet(t, cond, No)
		b, _ := runSearchIsSet(t, builtin, No)
		if a != b {
			t.Errorf("arr[%s]: `[[ -v ]]` said %q and `test -v` said %q", sub, a, b)
		}
	}
}
