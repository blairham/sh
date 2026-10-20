// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// The trace attribute's word, and where it stands in the sequence.
//
// `typeset -t v=1` was `scalar` here where the reference says `scalar-tag`,
// and the letter was not being ignored: `typeset -p v` already wrote
// `typeset -t v=1` back, byte for byte, which is what said the attribute was
// recorded and only the description could not reach it (#4872). Measured
// 2026-09-27 on zsh 5.9.2 under `-f` from a script file; see
// dialect/zsh/parameters.go for the position and how it was pinned.
func TestTheTraceAttributeHasAWord(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the headline", `typeset -t v=1; print -r -- ${(t)v}`, "scalar-tag\n"},
		{"inside a function", `f(){ typeset -t w=1; print -r -- ${(t)w} }; f`, "scalar-local-tag\n"},
		{"an array carries it", `typeset -ta a=(1 2); print -r -- ${(t)a}`, "array-tag\n"},
		{"an integer carries it", `typeset -ti n=1; print -r -- ${(t)n}`, "integer-tag\n"},
		{"the plus form takes it off", `typeset -t u=1; typeset +t u; print -r -- ${(t)u}`, "scalar\n"},

		// The position, one pair at a time. Each row holds `-t` fixed and
		// moves the letter beside it, which is what says this is one place
		// in the sequence rather than a rule per pair.
		{"after a left width", `typeset -t -L5 v=1; print -r -- ${(t)v}`, "scalar-left-tag\n"},
		{"after a zero fill", `typeset -t -Z5 v=1; print -r -- ${(t)v}`, "scalar-right_zeros-tag\n"},
		{"after the lower case letter", `typeset -t -l v=AB; print -r -- ${(t)v}`, "scalar-lower-tag\n"},
		{"after the upper case letter", `typeset -t -u v=ab; print -r -- ${(t)v}`, "scalar-upper-tag\n"},
		{"after the freeze", `typeset -t -r v=1; print -r -- ${(t)v}`, "scalar-readonly-tag\n"},
		{"before the export", `typeset -t -x v=1; print -r -- ${(t)v}`, "scalar-tag-export\n"},
		{"before unique", `typeset -t -U -a v=(1); print -r -- ${(t)v}`, "array-tag-unique\n"},
		{"before the two hiding words", `typeset -t -h -H v=1; print -r -- ${(t)v}`, "scalar-tag-hide-hideval\n"},
		{"before a tie and what follows it", `export PATH; typeset -t PATH; typeset -t path; print -r -- ${(t)PATH} ${(t)path}`, "scalar-tag-tied-export-special array-tag-tied-special\n"},
		{
			"and all of it at once",
			`typeset -t -x -r -u -H -h -L5 v=1; print -r -- ${(t)v}`,
			"scalar-left-upper-readonly-tag-export-hide-hideval\n",
		},

		// And through the other declaration word, which #4853 made one:
		// the letter was accepted there and recorded nothing, and with that
		// widening landed these three rows come out of the same table as
		// the rest.
		{"readonly carries it too", `readonly -t rr=2; print -r -- ${(t)rr}`, "scalar-readonly-tag\n"},
		{"beside another of its letters", `readonly -tu ru=ab; print -r -- ${(t)ru}`, "scalar-upper-readonly-tag\n"},
		{"and its listing writes both letters", `readonly -t rp=2; typeset -p rp`, "typeset -rt rp=2\n"},

		// The controls. A neighboring letter puts its word in correctly, so
		// this is not "no attribute letter reaches a word"; and the listing
		// was right before any of this, which is what made the gap a
		// description's rather than an attribute's.
		{"a neighboring letter was already right", `typeset -u c=ab; print -r -- ${(t)c}`, "scalar-upper\n"},
		{"the listing still writes the letter", `typeset -t p=1; typeset -p p`, "typeset -t p=1\n"},
		{"and a name without the letter has no word", `typeset q=1; print -r -- ${(t)q}`, "scalar\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}
