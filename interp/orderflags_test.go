// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// ordering is the grammar these expansions need, named by the constructs
// rather than by a shell.
func ordering(d *syntax.Dialect) {
	d.ParamExpansionFlags = true
	d.ArrayLiteral = true
	d.ArraySubscript = true
}

// The flags that decide which words survive and in what order.
//
// Every row is a measurement on zsh 5.9.2 under `LC_ALL=C`, which is what
// the corpus runs in — and the locale matters here in a way it does not for
// the other flags: outside C, `(o)` orders by the locale's collation and
// `(B a C b)` comes back `a b B C` rather than `B C a b`. That is recorded
// in the spec and not implemented.
func TestTheOrderingFlags(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"o sorts ascending", `a=(c a b); printf "[%s]" "${(@o)a}"`, "[a][b][c]"},
		{"O sorts descending", `a=(c a b); printf "[%s]" "${(@O)a}"`, "[c][b][a]"},
		// The row that tells the two sorts apart, and the reason single
		// digits are no use as data: lexically 10 comes before 9.
		{"o is lexical", `a=(10 9 1); printf "[%s]" "${(@o)a}"`, "[1][10][9]"},
		{"n is numeric", `a=(10 9 1); printf "[%s]" "${(@n)a}"`, "[1][9][10]"},
		{"and the digits need not be the whole word", `a=(x1 x10 x9); printf "[%s]" "${(@n)a}"`, "[x1][x9][x10]"},
		{"nor at the end of it", `a=(a1b a10b a2b); printf "[%s]" "${(@n)a}"`, "[a1b][a2b][a10b]"},
		{"a numeric tie falls back to the text", `a=(001 1 01); printf "[%s]" "${(@n)a}"`, "[001][01][1]"},
		{"a sign is not part of the number", `a=(-1 2 -3); printf "[%s]" "${(@n)a}"`, "[-1][-3][2]"},
		{"O reverses the numeric sort too", `a=(10 9 1); printf "[%s]" "${(@nO)a}"`, "[10][9][1]"},
		{"and the letters may be written either way round", `a=(10 9 1); printf "[%s]" "${(@On)a}"`, "[10][9][1]"},
		// Case. Byte order separates the cases; `i` folds them, and the tie
		// it makes reachable keeps the order the elements were written in.
		{"o is byte order, so case separates", `a=(B a C b); printf "[%s]" "${(@o)a}"`, "[B][C][a][b]"},
		{"i folds the case", `a=(B a C b); printf "[%s]" "${(@oi)a}"`, "[a][B][b][C]"},
		{"a fold makes a tie, and four elements agree", `a=(b B); printf "[%s]" "${(@oi)a}"`, "[b][B]"},
		{"i reverses with O like the rest", `a=(B a C b); printf "[%s]" "${(@Oi)a}"`, "[C][B][b][a]"},
		{"i sorts on its own", `a=(c a b); printf "[%s]" "${(@i)a}"`, "[a][b][c]"},
		{"i folds the numeric sort as well", `a=(B10 a9 A2); printf "[%s]" "${(@ni)a}"`, "[A2][a9][B10]"},
		{"and n alone does not", `a=(B10 a9 A2); printf "[%s]" "${(@n)a}"`, "[A2][B10][a9]"},
		// `u` is not a sort. It keeps the first of each repeat and leaves
		// the order alone, which is what fixes it ahead of the sort when
		// both are written.
		{"u keeps the first of each repeat", `a=(b a b c a); printf "[%s]" "${(@u)a}"`, "[b][a][c]"},
		{"u does not fold", `a=(a A a); printf "[%s]" "${(@u)a}"`, "[a][A]"},
		{"nor with i, which only sorts", `a=(a A a); printf "[%s]" "${(@ui)a}"`, "[a][A]"},
		{"u with a sort", `a=(b a b c a); printf "[%s]" "${(@ou)a}"`, "[a][b][c]"},
		{"either way round", `a=(b a b c a); printf "[%s]" "${(@uo)a}"`, "[a][b][c]"},
		{"and descending", `a=(b a b c a); printf "[%s]" "${(@uO)a}"`, "[c][b][a]"},
		// Where the step sits. Every one of these is a measurement and none
		// is what the manual's rule numbers suggest by name.
		{"the operator runs first", `a=(zb ya); printf "[%s]" "${(@o)a#z}"`, "[b][ya]"},
		{"and so does the case conversion", `a=(B a); printf "[%s]" "${(@oU)a}"`, "[A][B]"},
		{"a split makes the list the sort acts on", `v=c,a,b; printf "[%s]" "${(@s.,.o)v}"`, "[a][b][c]"},
		// And a join has already made one word, which is in order however it
		// was written.
		{"a forced join leaves nothing to sort", `a=(c a b); printf "[%s]" "${(oj.-.)a}"`, "[c-a-b]"},
		{"as does the quoted join without @", `a=(c a b); printf "[%s]" "${(o)a}"`, "[c a b]"},
		// And the two rows that moved the step: it is the *last* thing the
		// group does, after the prompt escapes and after the quoting, not
		// before them. `*` sorts ahead of a path only once `%x` has become
		// one, and `a!` ahead of `a\ b` only once the space has become a
		// backslash — both reverse if the sort runs first.
		{"the quoting has already run", `a=("a b" "a!"); printf "[%s]" "${(@qo)a}"`, `[a!][a\ b]`},
		{"and the letters may be written either way round here too", `a=("a b" "a!"); printf "[%s]" "${(@oq)a}"`, `[a!][a\ b]`},
		{"and so has the unquoting", `a=("'z'" b); printf "[%s]" "${(@Qo)a}"`, "[b][z]"},
		// `a` is the sort *key* and not a sort: the element's own position,
		// so ascending is the order it was written in. Which is the reading
		// the issue's own list would have got wrong — it grouped `a` with
		// `A` and `e` as flags that change the *subject* rather than the
		// order, and measurement puts it here instead.
		{"a orders by the index, which is the order written", `a=(c a b); printf "[%s]" "${(@a)a}"`, "[c][a][b]"},
		{"and O reverses that", `a=(c a b); printf "[%s]" "${(@aO)a}"`, "[b][a][c]"},
		{"either way round", `a=(c a b); printf "[%s]" "${(@Oa)a}"`, "[b][a][c]"},
		// The rows that say `a` *wins* rather than merely joining in. Each
		// of the three comparisons would give `a b c` on this data, so a
		// reading where `o` still decided would pass none of them.
		{"a beats o", `a=(c a b); printf "[%s]" "${(@oa)a}"`, "[c][a][b]"},
		{"a beats n", `a=(c a b); printf "[%s]" "${(@na)a}"`, "[c][a][b]"},
		{"a beats i", `a=(c a b); printf "[%s]" "${(@ia)a}"`, "[c][a][b]"},
		{"and beats them descending too", `a=(c a b); printf "[%s]" "${(@aOn)a}"`, "[b][a][c]"},
		{"u still runs ahead of it", `a=(b a b c a); printf "[%s]" "${(@au)a}"`, "[b][a][c]"},
		{"a scalar has one index", `v=hello; printf "[%s]" "${(a)v}"`, "[hello]"},
		{"and an empty array none", `a=(); printf "[%s]" "${(@a)a}"`, "[]"},
		{"a scalar is already in order", `v=cab; printf "[%s]" "${(o)v}"`, "[cab]"},
		{"an empty array stays empty", `a=(); printf "[%s]" "${(@o)a}"`, "[]"},
		{"and one element stays one", `a=(one); printf "[%s]" "${(@o)a}"`, "[one]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, ordering, nil)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The order of a tie is this implementation's choice and not a measurement.
//
// The shell with the construct does not order equal elements predictably: on
// four it keeps the order they were written in, and on sixteen of the same
// shape it reverses some of the pairs and not others — its sort algorithm
// showing through. So the corpus pins the sizes where the two agree, and this
// pins what we do at a size where they do not: a stable sort, because a
// deterministic answer is worth more than an unpredictable match.
//
// Sixteen is not arbitrary. Go's unstable sort is its stable one below a
// threshold, so a smaller array could not tell the two apart and the claim
// would be untested.
func TestATieKeepsTheOrderItWasWrittenIn(t *testing.T) {
	const src = `a=(B1 b1 B2 b2 B3 b3 B4 b4 B5 b5 B6 b6 B7 b7 A1 a1); printf "[%s]" "${(@oi)a}"`
	want := "[A1][a1][B1][b1][B2][b2][B3][b3][B4][b4][B5][b5][B6][b6][B7][b7]"
	out, st := runGrammar(t, src, ordering, nil)
	if out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, want)
	}
}
