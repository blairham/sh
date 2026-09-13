// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// A subscript written at command position runs to its matching `]`, so a key
// may hold a blank (#2410). Measured against bash 5.3.15 on 2026-09-12, every
// row read back with `typeset -p m`.
func TestASubscriptAtCommandPositionHoldsItsSeparators(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			// The issue's own line.
			"a blank",
			`typeset -A m; m[foo bar]=qux; echo "[${m[foo bar]}]"`,
			"[qux]\n",
		},
		{
			// The same cut showed on the append spelling.
			"an append",
			`typeset -A m; m[foo bar]=qux; m[foo bar]+=" blat"; echo "[${m[foo bar]}]"`,
			"[qux blat]\n",
		},
		{
			// Not blanks but brackets: an operator inside them is a
			// character of the key.
			"a semicolon",
			`typeset -A m; m[a; b]=v; echo "[${m[a; b]}]"`,
			"[v]\n",
		},
		{
			// It is the *matching* `]`, so brackets nest.
			"a nested bracket",
			`typeset -A m; m[a [b] c]=v; echo "[${m[a [b] c]}]"`,
			"[v]\n",
		},
		{
			// An assignment prefix is command position too.
			"behind another assignment",
			`typeset -A m; a=1 m[foo bar]=v; echo "[${m[foo bar]}]$a"`,
			"[v]1\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runBash(t, t.TempDir(), c.src)
			if out != c.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, c.want)
			}
		})
	}
}

// An argument is not command position, and bash ends one at the blank: the
// two fields are what `printf` is handed. Without this row the grammar flag
// would read as "a subscript holds its blanks", which is a larger claim than
// the shell makes.
func TestAnArgumentsSubscriptStillEndsAtTheBlank(t *testing.T) {
	out, st := runBash(t, t.TempDir(), `printf "<%s>" m[foo bar]=v; echo`)
	const want = "<m[foo><bar]=v>\n"
	if out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, want)
	}
}

// The same reading in the other position: an element of a compound array
// literal that opens with `[` runs to its matching `]` (#2299).
//
// Measured against bash 5.3.15 on 2026-09-13, each row read back through the
// key rather than through `typeset -p`, because what went wrong was the key:
// the element was stored as `[two` holding `words]=2`, so nothing failed and
// the array simply held something nobody wrote.
func TestAnArrayLiteralElementHoldsItsSubscriptsSeparators(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			// The issue's own line.
			"a blank",
			`typeset -A m; m=( [one]=1 [two words]=2 ); echo "[${m[two words]}][${m[one]}]"`,
			"[2][1]\n",
		},
		{
			// `[2x]` and not `[x]`: measured, a `+=` element in a plain
			// `m=( … )` appends to the element that was there, which is the
			// reading pinned in #2405. The row is here for the *subscript*
			// and takes the value bash gives it.
			"an append",
			`typeset -A m; m=( [two words]=2 ); m=( [two words]+=x ); echo "[${m[two words]}]"`,
			"[2x]\n",
		},
		{
			"a semicolon",
			`typeset -A m; m=( [a; b]=v ); echo "[${m[a; b]}]"`,
			"[v]\n",
		},
		{
			"a nested bracket",
			`typeset -A m; m=( [a [b] c]=v ); echo "[${m[a [b] c]}]"`,
			"[v]\n",
		},
		{
			// The value after the subscript is still cut at the blank: only
			// the bracketed text spans, so this is two elements.
			"the value is not spanned",
			`m=( [0]=a b ); echo "${#m[@]}"`,
			"2\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runBash(t, t.TempDir(), c.src)
			if out != c.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, c.want)
			}
		})
	}
}

// The bracket has to be the front of the element, and bash says so three
// ways: a name in front of it, a value that merely holds one, and a quoted
// one are all still cut at the blank. Measured 2026-09-13 on bash 5.3.15.
func TestOnlyAnElementsOwnFrontOpensASpanningSubscript(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`a=( pre[1 2]=x ); printf "<%s>" "${a[@]}"; echo`, "<pre[1><2]=x>\n"},
		{`a=( x[1 2] ); printf "<%s>" "${a[@]}"; echo`, "<x[1><2]>\n"},
		{`a=( "[1 2]"=x ); printf "<%s>" "${a[@]}"; echo`, "<[1 2]=x>\n"},
	} {
		out, st := runBash(t, t.TempDir(), c.src)
		if out != c.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", c.src, out, st, c.want)
		}
	}
}
