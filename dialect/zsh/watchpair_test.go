// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `$WATCH` and `$watch` are one parameter in two kinds, and the reference
// does not call it `tied`.
//
// Both halves of that sentence are measured, and the second is what #4907
// asked for before any code: implementing the pair through the built-in tie
// — which is what the behavior asks for — would have answered
// `scalar-tied-special` where the reference answers `scalar-special`, trading
// one wrong word for another. See watchpair.go, where the third measurement
// that settles it is: `typeset +T` there names the eight tied pairs and
// `ZSH_EVAL_CONTEXT`, and neither half of this one.
//
// Measured 2026-09-27 against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `go version -m` says *not a Go executable*
// for it — under `-f` from a script file with `env -i PATH=/usr/bin:/bin
// TERM=dumb` and a scratch `HOME`.
func TestTheWatchPairIsJoinedAndNotTied(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			// The join, both ways round, in one shell.
			"the array half writes the scalar",
			`watch=(a b); print -r -- "[$WATCH]"`,
			"[a:b]\n",
		},
		{
			"and the scalar half writes the array",
			`WATCH=cc; print -r -- "n=${#watch} [${(j:,:)watch}]"`,
			"n=1 [cc]\n",
		},
		{
			// An empty field is a field, which is what says the split is on
			// the separator rather than on runs of it.
			"an empty field between two separators is an element",
			`WATCH='a:b::c'; print -r -- "n=${#watch} [${(j:|:)watch}]"`,
			"n=4 [a|b||c]\n",
		},
		{
			// The words, which are the whole of why this is not a tie.
			"neither half carries the tie word",
			`print -r -- "${(t)WATCH} / ${(t)watch}"`,
			"scalar-special / array-special\n",
		},
		{
			// The control in the same run: a pair that *is* a tie carries
			// the word, so the row above is a statement about this pair
			// rather than about a shell that has stopped writing `tied`.
			"and the shell's own tie still does",
			`print -r -- "${(t)PATH} / ${(t)path}"`,
			"scalar-tied-special / array-tied-special\n",
		},
		{
			// And no tie listing walks it, which is the measurement that
			// separates the join from the word: the pair mirrors and is not
			// in the table `typeset -T` writes into.
			"and the tie listing names neither half",
			`typeset +T | while IFS= read -r l; do case $l in (WATCH|watch) print -r -- "[$l]";; esac; done
			 print -r -- done`,
			"done\n",
		},
		{
			"while it does name the tied ones",
			`typeset +T | while IFS= read -r l; do case $l in (PATH|path) print -r -- "[$l]";; esac; done`,
			"[PATH]\n[path]\n",
		},
		{
			// Both halves are there and both are empty, which a
			// registration alone would not have said.
			"both halves are there and hold nothing",
			`print -r -- "${+WATCH}${+watch} [$WATCH] n=${#watch}"`,
			"11 [] n=0\n",
		},
		{
			// And both arrive on a first reference, which is measured
			// rather than assumed from their being specials: a bare
			// `typeset` in a shell that has referred to nothing writes them
			// under the word for that state.
			"and both are waiting for a first reference",
			`typeset | while IFS= read -r l; do case $l in (*WATCH|*watch) print -r -- "[$l]";; esac; done`,
			"[undefined WATCH]\n[undefined watch]\n",
		},
		{
			// The pattern anchors the name, which it did not need to until
			// #4998: a read of the pair loads `zsh/watch`, and this shell's
			// own record of the loaded modules is a parameter whose *value*
			// then holds the string `watch`. That record is visible to a
			// bare `typeset` here and to nothing at all in the reference,
			// which is a divergence of its own and is filed rather than
			// worked around — see #5014. What this row is about is the
			// pair's listing, so it asks for lines that *name* `watch`.
			"which a read takes them out of",
			`: ${#watch}
			 typeset | while IFS= read -r l; do case $l in (*[' ']watch=*) print -r -- "[$l]";; esac; done`,
			"[array watch=(  )]\n",
		},
		{
			// The `-p` form writes nothing for either until then, which is
			// the deferral's own row and is here because these two are the
			// only names in that roster no module brings.
			"and the -p listing writes nothing for either",
			`typeset -p WATCH watch; print -r -- "st=$?"`,
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
