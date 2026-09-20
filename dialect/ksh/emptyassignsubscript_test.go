// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/syntax"
)

// `a[]=6` — a plain assignment whose brackets hold nothing at all — is refused
// by the **grammar** here, and that is measured rather than assumed: the
// refusal arrives for text the shell never runs.
//
// Measured 2026-09-20 against ksh93u+ 2012-08-01, `env -i PATH=/usr/bin:/bin
// LC_ALL=C ksh k.sh` over a script file with standard input on the null
// device. `echo before ⏎ if false; then a[]=6; fi ⏎ echo after` writes
// `before`, then “k.sh: syntax error at line 2: `[]' empty subscript“, and
// exits 3 with no `after` — so the branch that is not taken is refused all the
// same, which the other two columns do not do: bash and zsh both run that
// script to the end in silence and complain only where the assignment is
// reached.
//
// Before this the subscript was dropped while the word was parsed, so the
// assignment arrived as a bare `a=6` — element **zero** of the array — at
// status 0 with nothing said (#3949).
func TestAnEmptyAssignmentSubscriptIsASyntaxError(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"a[]=6\n", "syntax error at line 1: `[]' empty subscript"},
		{"a=(1 2 3)\na[]=6\n", "syntax error at line 2: `[]' empty subscript"},
		{"u[]=6\n", "syntax error at line 1: `[]' empty subscript"},
		{"b[]+=6\n", "syntax error at line 1: `[]' empty subscript"},
		{"typeset -A m\nm[]=9\n", "syntax error at line 2: `[]' empty subscript"},
		// The branch that is never taken, which is what says this is the
		// grammar's answer and not the assignment's.
		{
			"echo before\nif false; then a[]=6; fi\necho after\n",
			"syntax error at line 2: `[]' empty subscript",
		},
		{"f() { a[]=6; }\n", "syntax error at line 1: `[]' empty subscript"},
		// Every link of a chain and not only the last, which this grammar
		// is the only one to have: measured, both spellings get the same
		// sentence.
		{"a[][2]=6\n", "syntax error at line 1: `[]' empty subscript"},
		{"a[2][]=6\n", "syntax error at line 1: `[]' empty subscript"},
		// A prefix assignment carries the brackets too.
		{"a[]=6 true\n", "syntax error at line 1: `[]' empty subscript"},
	} {
		_, err := syntax.Parse(tc.src, ksh.Dialect())
		if err == nil {
			t.Errorf("%q parsed, want a refusal", tc.src)
			continue
		}
		if got := ksh.Diagnostics().ParseFailure(err); got != tc.want {
			t.Errorf("%q:\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

// The sentence the front end writes for it, and the status behind it.
//
// This shell parses incrementally, so `echo before` on the line above has
// already run by the time the refusal arrives — the end-to-end shape was
// compared against ksh93u+ separately and agrees character for character,
// and what is asserted here is the sentence, its line, and the 3.
func TestTheSentenceAndStatusOfAnEmptyAssignmentSubscript(t *testing.T) {
	const src = "echo before\na=(1 2 3)\na[]=6\necho after\n"
	_, err := syntax.Parse(src, ksh.Dialect())
	if err == nil {
		t.Fatalf("%q parsed, want a refusal", src)
	}
	got := ksh.Diagnostics().ParseDiagnostic("ksh", "-c", err, src)
	if want := "ksh: syntax error at line 3: `[]' empty subscript\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if st := ksh.Diagnostics().StatusForParseError(err); st != 3 {
		t.Errorf("status %d, want 3", st)
	}
}

// The neighbors that must not have moved. Brackets holding *something* are a
// subscript, and each of these writes element zero at status 0 — which is the
// answer the empty brackets used to give, so a refusal that reached any of
// them would be this fix in the wrong place.
func TestTheSubscriptsThatAreNotWrittenEmpty(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`c=(1 2 3); c[""]=6; echo "st=$? c=[${c[@]}]"`, "st=0 c=[6 2 3]\n"},
		{`e=(1 2 3); i=; e[$i]=6; echo "st=$? e=[${e[@]}]"`, "st=0 e=[6 2 3]\n"},
		{`g=(1 2 3); g[0]=6; echo "st=$? g=[${g[@]}]"`, "st=0 g=[6 2 3]\n"},
		// A chain whose links all hold something is the nesting this
		// grammar is for, and it still parses.
		{`h[1][2]=v; typeset -p h`, "typeset -a h=([1]=([2]=v) )\n"},
	} {
		out, st := runKsh(t, t.TempDir(), c.src)
		if out != c.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", c.src, out, st, c.want)
		}
	}
}
