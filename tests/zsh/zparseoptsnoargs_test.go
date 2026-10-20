// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `zparseopts` with no arguments says `not enough arguments`, which is a
// different sentence from the one an empty description list gets.
//
// It was `missing option descriptions` at the same status, so what a script
// saw was the wording alone — and `V12zparseopts.ztst` compares wordings
// (#5008).
//
// Measured 2026-09-28 against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `go version -m` says *not a Go executable* for
// it — under `-f` from a script file with `env -i PATH=/usr/bin:/bin
// TERM=dumb` and a scratch `HOME`.
//
// **The rows below are chosen to kill the three readings the issue named**,
// because "nothing was left after the letters" is the obvious one and it is
// wrong: `-D` and `-a o` leave nothing and get the *other* sentence.
func TestZparseoptsWithNoArguments(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		{
			"no words at all",
			`zparseopts`, "zsh:zparseopts:1: not enough arguments\n", 1,
		},
		{
			// The end-of-options marker alone answers the same way, which
			// is what rules out "the argument list was empty": it is not.
			"the end-of-options marker alone",
			`zparseopts --`, "zsh:zparseopts:1: not enough arguments\n", 1,
		},
		{
			// And these are the controls that keep the sentence off the
			// other emptiness: a letter was given, nothing is left, and the
			// answer is the older sentence.
			"a letter with nothing after it keeps the other sentence",
			`zparseopts -D`, "zsh:zparseopts:1: missing option descriptions\n", 1,
		},
		{
			"and so does an array with nothing after it",
			`zparseopts -a o`, "zsh:zparseopts:1: missing option descriptions\n", 1,
		},
		{
			"and two letters",
			`zparseopts -D -E`, "zsh:zparseopts:1: missing option descriptions\n", 1,
		},
		{
			// A word that is not one of this builtin's letters is a
			// description, so it reaches the description reader and is
			// refused there — which is the third sentence and says the new
			// one has not been widened over it.
			"a description with no array is still refused at the description",
			`zparseopts a`, "zsh:zparseopts:1: no default array defined: a\n", 1,
		},
		{
			// The letters do not stack, so `-DE` is a description too.
			"and an unstacked pair is a description",
			`zparseopts -DE`, "zsh:zparseopts:1: no default array defined: -DE\n", 1,
		},
		{
			// The control on the other side: a call with a real description
			// still works, which is what says none of this reaches an
			// ordinary use.
			"a real description is unaffected",
			`set -- -o v rest
			 zparseopts -D o:=arr
			 print -r -- "st=$? arr=[${(j:,:)arr}] rest=[$*]"`,
			"st=0 arr=[-o,v] rest=[rest]\n", 0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZshPrelude(t, t.TempDir(), tc.src)
			if out != tc.want || st != tc.status {
				t.Errorf("%s = %q (status %d), want %q at %d", tc.src, out, st, tc.want, tc.status)
			}
		})
	}
}
