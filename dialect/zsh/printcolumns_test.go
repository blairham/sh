// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `print -C n` writes the operands in n columns, filled down.
//
// It was `-C is not implemented yet` at 1 — the first failing chunk of
// `B03print.ztst` (#4967).
//
// Measured 2026-09-28 against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `go version -m` says *not a Go executable* for
// it — under `-f` from a script file with `env -i PATH=/usr/bin:/bin
// TERM=dumb` and a scratch `HOME`.
//
// **The rows are chosen so the obvious readings part company.** The width rule
// looks like "the longest word plus one" from a single layout where three
// different rules agree, which is what the issue that filed this carried, and
// four of the rows below are there to rule one out each.
func TestPrintInColumns(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			// Filled **down**: the first column takes the first ceil(n/c)
			// words rather than the first row.
			"the columns are filled down",
			"print -C 2 a b c d",
			"a  c\nb  d\n",
		},
		{
			// Every word the same length, so the width is one plus two.
			// This is the row all three readings agree on.
			"the padded field is the longest counted word plus two",
			"print -C 2 aaaa bbbb cccc dddd",
			"aaaa  cccc\nbbbb  dddd\n",
		},
		{
			// **Not the longest word in the list**: `dddd` is four and the
			// width is five, because it sits in the last column and the
			// last column is never counted.
			"the last column does not count toward the width",
			"print -C 2 aaa b cc dddd",
			"aaa  cc\nb    dddd\n",
		},
		{
			// **Not the longest word in each column** either: column one
			// holds `1` and `22` and is six wide, because `4444` in column
			// two is four. One width for the whole layout.
			"and the width is one number for the whole layout",
			"print -C 3 1 22 333 4444 55555",
			"1     333   55555\n22    4444\n",
		},
		{
			// `4444` above is also the row that separates *counted* from
			// *padded*: it is the last field of its row, so it is written
			// with nothing after it, and it still decides the width.
			"the last field of a row is never padded",
			"print -C 2 a b c",
			"a  c\nb\n",
		},
		{
			// More columns than words is one row, and needs no second rule:
			// `a` is padded, `b` is not, so the width is 1+2. The issue
			// left this open as an edge the "plus one" reading could not
			// explain.
			"more columns than words is one row",
			"print -C 5 a b",
			"a  b\n",
		},
		{
			"one column is one word to a line",
			"print -C 1 a b",
			"a\nb\n",
		},
		{
			"and a single word is written alone",
			"print -C 2 onlyone",
			"onlyone\n",
		},
		{
			// An empty operand is a field like any other, and is padded.
			"an empty operand is a field",
			`print -C 2 "" b c d`,
			"   c\nb  d\n",
		},
		{
			// The layout wins over the separators, which is what makes `-C`
			// a layout rather than a join.
			"the layout wins over -l",
			"print -l -C 2 a b c",
			"a  c\nb\n",
		},
		{
			// What `-N` still decides is the **row terminator**, and there
			// is no trailing newline after it.
			"and -N still ends every row",
			"print -N -C 2 a b c d",
			"a  c\x00b  d\x00",
		},
		{
			// `-n` decides nothing here: the rows keep their separators.
			"where -n decides nothing",
			"print -n -C 2 a b c d",
			"a  c\nb  d\n",
		},
		{
			// The sort happens first, so `-o` reorders the words and the
			// layout runs on the result.
			"a sort happens before the layout",
			"print -o -C 2 d c b a",
			"a  c\nb  d\n",
		},
		{
			// No operands is no rows at all, at 0.
			"no operands writes nothing",
			`print -C 2; print -r -- "st=$?"`,
			"st=0\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZshPrelude(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// The width is a **display width** of the processed text, and a control
// character has none of it.
//
// Its own test because it is three separate claims and each has a row that
// fails without it: the escapes are expanded before the measuring, a tab takes
// no room, and a wide character takes two. Measured the same day and the same
// way, each first operand against the same `cc dd ee`.
func TestPrintInColumnsMeasuresTheProcessedWidth(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			// The control: three ordinary characters, three plus two.
			"an ordinary word is its characters",
			"print -C 2 xyz cc dd ee",
			"xyz  dd\ncc   ee\n",
		},
		{
			// The escapes are expanded and *then* measured: `a\tb` is four
			// characters as written and the layout is four wide, which is
			// the two printable characters of `a<TAB>b` plus two.
			"a tab is expanded and then counts nothing",
			`print -C 2 'a\tb' cc dd ee`,
			"a\tb  dd\ncc  ee\n",
		},
		{
			// And with `-r` the backslash and the `t` are two characters
			// that do count, so the same line is six wide. The pair is what
			// says the measuring is of the processed text.
			"where the raw spelling counts both characters",
			`print -r -C 2 'a\tb' cc dd ee`,
			"a\\tb  dd\ncc    ee\n",
		},
		{
			// A wide character takes two columns, which rules out the
			// character count.
			"a wide character takes two columns",
			"print -C 2 你好 cc dd ee",
			"你好  dd\ncc    ee\n",
		},
		{
			// And an accented letter takes one, which rules out the byte
			// count.
			"and an accented letter takes one",
			"print -C 2 é bb cc dd",
			"é   cc\nbb  dd\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZshPrelude(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// And the two refusals the operand can make, which are worded differently.
func TestPrintInColumnsRefusesABadCount(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"zero columns", "print -C 0 a b", "zsh:print:1: invalid number of columns: 0\n"},
		{"a negative count", "print -C -1 a b", "zsh:print:1: invalid number of columns: -1\n"},
		{"and a word that is not a number", "print -C x a b", "zsh:print:1: number expected after -C: x\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZshPrelude(t, t.TempDir(), tc.src)
			if out != tc.want || st != 1 {
				t.Errorf("%s = %q (status %d), want %q at 1", tc.src, out, st, tc.want)
			}
		})
	}
}
