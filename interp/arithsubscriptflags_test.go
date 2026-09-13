// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// runSubArith runs src with the grammar a subscript flag group needs and the
// arithmetic that reads one, which is the same pair the expansion suite turns
// on plus nothing.
func runSubArith(t *testing.T, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, func(d *syntax.Dialect) {
		subGrammar(d)
		d.ArrayLiteral = true
	}, func(r *Runner) {
		sem := *r.Semantics
		sem.ArrayBaseIsZero = No
		sem.GlobExpansionResults = No
		sem.SplitParamExpansion = No
		sem.GlobNoMatchIsError = Yes
		sem.SubscriptIsAQuotingContext = No
		r.Semantics = &sem
	})
}

// A subscript's flag group is read inside an **expression** too, by the same
// machinery an expansion reads it with: `$(( a[(r)20] ))` selects what
// `${a[(r)20]}` selects, and the answer is then read as a number the way every
// other element's value is.
//
// The arithmetic route did not consult the group at all. It handed the whole
// text — group and operand together — to the expression reader on an indexed
// name, which refused it, and to the table as a *key* on an association, which
// found nothing and answered a silent zero (#1986).
func TestAFlagGroupIsReadInsideAnExpression(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the value a search found", `a=(10 20 30); echo $(( a[(r)20] ))`, "20"},
		{"the index it found it at", `a=(10 20 30); echo $(( a[(i)20] ))`, "2"},
		{"the last match", `a=(10 20 30); echo $(( a[(I)2*] ))`, "2"},
		{"an index miss, one past the end", `a=(10 20 30); echo $(( a[(i)99] ))`, "4"},
		{"a value miss, whose value is nothing", `a=(10 20 30); echo $(( a[(r)99] ))`, "0"},
		{
			"a group that selects nothing is an ordinary subscript",
			`a=(10 20 30); echo $(( a[(e)2] ))`, "20",
		},
		{"an operand like any other", `a=(10 20 30); echo $(( a[(r)20] + 1 ))`, "21"},
		{"a table, read by its key", `typeset -A m; m[k]=9; echo $(( m[(k)k] ))`, "9"},
		{"a table searched by value", `typeset -A m; m[k]=9; echo $(( m[(r)9] ))`, "9"},
		{
			"a table searched by key, which answers the key",
			`typeset -A m; m[7]=9; echo $(( m[(i)7] ))`, "7",
		},
		{"a character, which is no number", `s=hello; echo $(( s[(r)l] ))`, "0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := runSubArith(t, tc.src)
			if got := strings.TrimSpace(out); got != tc.want || status != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, got, status, tc.want)
			}
		})
	}
}

// And the **write** through one names the same element the read names, which
// is the whole of why the two go through one function: two rules would write
// where the read could not look.
func TestAFlagGroupNamesTheElementAWriteInsideAnExpressionReaches(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a value search", `a=(10 20 30); (( a[(r)20] = 9 )); printf "[%s]" "${a[@]}"`, "[10][9][30]"},
		{"an index search", `a=(10 20 30); (( a[(i)20] = 9 )); printf "[%s]" "${a[@]}"`, "[10][9][30]"},
		{"a step through one", `a=(10 20 30); (( a[(r)20]++ )); printf "[%s]" "${a[@]}"`, "[10][21][30]"},
		{"a group that selects nothing", `a=(10 20 30); (( a[(e)2] = 9 )); printf "[%s]" "${a[@]}"`, "[10][9][30]"},
		{"a miss, which appends", `a=(10 20 30); (( a[(r)99] = 9 )); printf "[%s]" "${a[@]}"`, "[10][20][30][9]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := runSubArith(t, tc.src)
			if out != tc.want || status != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, status, tc.want)
			}
		})
	}
}

// A search naming a place to *write* in a table writes nowhere and says
// nothing, which is the one shell with the construct measured rather than a
// shape chosen: 2026-09-12 on zsh 5.9.2, with `typeset -A m; m=(aa 1)`,
// `(( m[(r)1] = 5 ))`, `(( m[(k)aa] = 5 ))` and `(( m[(k)aa]++ ))` all leave
// the table exactly as it was at status 0, and `x=$(( m[(k)aa] = 5 ))` gives
// `x` the 5 while the table keeps its 1 — so the expression has its value and
// only the store is dropped.
//
// The same search on the left of a plain `=` is refused by name there, which
// is one shell giving two answers to what looks like one question and not
// something this engine reconciles.
//
// This route consulted the association before the group, so the whole
// subscript text became the key: `(( m[(r)1] = 5 ))` left a table holding a
// key literally named `(r)1`, silently, at status 0 — a plausible table with
// an element nobody wrote (#2288).
func TestASearchWritingATableInsideAnExpressionWritesNothing(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a value search", `typeset -A m; m[aa]=1; (( m[(r)1] = 5 )); printf "%d[%s]" "${#m[@]}" "${m[aa]}"`, "1[1]"},
		{"a key search", `typeset -A m; m[aa]=1; (( m[(k)aa] = 5 )); printf "%d[%s]" "${#m[@]}" "${m[aa]}"`, "1[1]"},
		{"a step through one", `typeset -A m; m[aa]=1; (( m[(k)aa]++ )); printf "%d[%s]" "${#m[@]}" "${m[aa]}"`, "1[1]"},
		{
			"the expression still has its value",
			`typeset -A m; m[aa]=1; x=$(( m[(k)aa] = 5 )); printf "[%s]%d[%s]" "$x" "${#m[@]}" "${m[aa]}"`,
			"[5]1[1]",
		},
		// The control: a group selecting nothing leaves an ordinary key
		// behind it, and that key is written — so the silence above is the
		// search's and not the group's.
		{
			"a group that selects nothing is a key",
			`typeset -A m; m[aa]=1; (( m[(e)aa] = 5 )); printf "%d[%s]" "${#m[@]}" "${m[aa]}"`,
			"1[5]",
		},
		{
			"and one naming a key that is not there makes it",
			`typeset -A m; m[aa]=1; (( m[(e)zz] = 5 )); printf "%d[%s]" "${#m[@]}" "${m[zz]}"`,
			"2[5]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := runSubArith(t, tc.src)
			if out != tc.want || status != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, status, tc.want)
			}
			if strings.Contains(out, "(r)") || strings.Contains(out, "(k)") {
				t.Errorf("%s = %q, want no key named after the group's own letters", tc.src, out)
			}
		})
	}
}

// A letter the group is read with and this does not carry is refused by name
// here as it is in an expansion, rather than becoming part of a key or of an
// expression.
func TestAFlagInAnExpressionThisDoesNotCarryIsRefusedByName(t *testing.T) {
	const src = `a=(10 20 30); echo $(( a[(w)20] ))`
	out, status := runSubArith(t, src)
	if !strings.Contains(out, "(w) subscript flag is not implemented") || status == 0 {
		t.Errorf("%s = %q (status %d), want a refusal naming (w)", src, out, status)
	}
}

// The parentheses are a group only where the grammar has them. Everywhere else
// they are text, and the subscript is read as the arithmetic it is — which is
// what keeps this additive.
func TestParenthesesInASubscriptAreTextWithoutTheGrammar(t *testing.T) {
	const src = `a=(10 20 30); echo $(( a[(r)20] ))`
	out, status := runGrammar(t, src, subscripting, nil)
	if status == 0 || strings.Contains(out, "subscript flag") {
		t.Errorf("%s = %q (status %d), want the arithmetic to read the brackets", src, out, status)
	}
}
