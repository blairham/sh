// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// positionalFlagGrammar is the grammar these rows need, named by the
// constructs: a subscript, a parameter that is not a name carrying one, the
// parenthesized flag group in front of it, and an expansion standing where a
// name would, for the nested spelling the last rows use. The flags are what
// make the bug visible — a group is the only thing that rewrites an
// expansion's words one element at a time — but the subject is the subscript,
// which is what reaches the storage.
func positionalFlagGrammar(d *syntax.Dialect) {
	d.ArraySubscript = true
	d.ArrayLiteral = true
	d.ParamExpansionFlags = true
	d.SpecialParamSubscript = true
	d.NestedParamExpansion = true
}

// runPositionalFlag runs src with that grammar and with the answers the rows
// need, none of which is the subject: a comma in a subscript is a range —
// without it two of the three spellings are not a construct at all —
// subscripts count from one, and nothing is split or globbed on the way out.
func runPositionalFlag(t *testing.T, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, positionalFlagGrammar, func(r *Runner) {
		sem := *r.Semantics
		sem.SubscriptCommaIsARange = Yes
		sem.ArrayBaseIsZero = No
		sem.ArrayNameWithoutSubscriptIsTheList = Yes
		sem.SplitParamExpansion = No
		sem.GlobExpansionResults = No
		sem.OperatorDistributesOverStarSubscript = No
		r.Semantics = &sem
	})
}

// Reading the positional parameters does not write to them.
//
// A subscript on `@` reaches the runner's own parameter storage, and a flag
// group rewrites an expansion's words one element at a time — so handing that
// storage out live let `${(q)@[1,-1]}` quote the parameters *themselves*. The
// next read then quoted what the previous read had already quoted, which is a
// backslash level added per round trip where the shell removes one (#1622).
//
// Every row reads twice or three times and asserts the whole result, because a
// single read cannot tell the two apart: one added level and one removed level
// look alike at one step, and only the second read shows which happened. The
// second half of each row is the parameter itself, which must be exactly what
// `set --` was given.
//
// Measured on zsh 5.9.2, 2026-09-09.
func TestReadingThePositionalParametersDoesNotWriteToThem(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a range leaves the parameter alone",
			`set -- 'a b'; printf "[%s]" "${(q)@[1,-1]}"; printf "[%s]" "$1"`,
			`[a\ b][a b]`,
		},
		{
			"and reads the same three times running",
			`set -- 'a b'; printf "[%s]" "${(q)@[1,-1]}" "${(q)@[1,-1]}" "${(q)@[1,-1]}"`,
			`[a\ b][a\ b][a\ b]`,
		},
		{
			"a whole-array subscript likewise",
			`set -- 'a b'; printf "[%s]" "${(q)@[@]}" "${(q)@[@]}"; printf "[%s]" "$1"`,
			`[a\ b][a\ b][a b]`,
		},
		{
			"and a range naming one element",
			`set -- 'a b'; printf "[%s]" "${(q)@[1,1]}" "${(q)@[1,1]}"; printf "[%s]" "$1"`,
			`[a\ b][a\ b][a b]`,
		},
		{
			"the star spelling is the same storage",
			`set -- 'a b'; printf "[%s]" "${(q)*[1,-1]}" "${(q)*[1,-1]}"; printf "[%s]" "$1"`,
			`[a\ b][a\ b][a b]`,
		},
		// Not a quoting flag, so the row says the subject is the storage and
		// not the `q` family: a case conversion writes through the same slice.
		{
			"a case conversion does not stick either",
			`set -- 'a b'; printf "[%s]" "${(U)@[1,-1]}" "${(U)@[1,-1]}"; printf "[%s]" "$1"`,
			`[A B][A B][a b]`,
		},
		// A range over the middle of the list, which is the shape the plugin
		// manager writes: `"${(j: :)${(q)@[2,-1]}}"`, once per item.
		{
			"a range past the first parameter leaves all of them alone",
			`set -- 'a b' 'c d' 'e f'; printf "[%s]" "${(j: :)${(q)@[2,-1]}}" "${(j: :)${(q)@[2,-1]}}"; printf "[%s]" "$1" "$2" "$3"`,
			`[c\ d e\ f][c\ d e\ f][a b][c d][e f]`,
		},
		// The unsubscripted spelling was already right, and a named array
		// always was. Both are here so a fix that traded one path for another
		// fails on this row rather than somewhere a reader has to go looking.
		{
			"the unsubscripted spelling was already right",
			`set -- 'a b'; printf "[%s]" "${(q)@}" "${(q)@}"; printf "[%s]" "$1"`,
			`[a\ b][a\ b][a b]`,
		},
		{
			"and a named array is unaffected",
			`n=('a b'); printf "[%s]" "${(q)n[1,-1]}" "${(q)n[1,-1]}"; printf "[%s]" "${n[1]}"`,
			`[a\ b][a\ b][a b]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runPositionalFlag(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}
