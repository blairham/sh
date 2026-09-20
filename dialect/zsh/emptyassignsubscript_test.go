// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `a[]=6` — a plain assignment whose brackets hold nothing at all — is refused
// as a name, nothing is written, and the script **ends**.
//
// Measured 2026-09-20 against zsh 5.9.2, `env -i PATH=/usr/bin:/bin LC_ALL=C
// zsh z.sh` over a script file with standard input on the null device: every
// shape below writes `z.sh:N: not an identifier: <name>[]` and the shell exits
// 1 with no line after it run. The sentence is the one this shell already
// writes for `(( a[] = 4 ))`, which is Diagnostics.ArithEmptySubscriptTarget.
//
// Before this the subscript was dropped while the word was parsed, so the
// assignment arrived as a bare `a=6` — which in this dialect replaces the
// whole array with a scalar — at status 0 with nothing said (#3949).
func TestAnEmptyAssignmentSubscriptEndsTheScript(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`a=(1 2 3); a[]=6; echo unreached`, "zsh:1: not an identifier: a[]\n"},
		{`u[]=6; echo unreached`, "zsh:1: not an identifier: u[]\n"},
		{`b=(1 2 3); b[]+=6; echo unreached`, "zsh:1: not an identifier: b[]\n"},
		{`typeset -A m; m[k]=v; m[]=9; echo unreached`, "zsh:1: not an identifier: m[]\n"},
	} {
		out, st := runZsh(t, t.TempDir(), c.src)
		if out != c.want || st != 1 {
			t.Errorf("%s = %q (status %d), want %q at 1", c.src, out, st, c.want)
		}
	}
}

// Nothing is written and nothing after it runs, which is the half a check on
// the sentence alone would miss: the array is read back in a *later* shell,
// since this one does not survive to print it.
func TestAnEmptyAssignmentSubscriptWritesNothing(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		// A subshell takes the refusal, so the outer script lives to say
		// what the name holds.
		{
			`a=(1 2 3); ( a[]=6 ); echo "a=[${a[@]}] n=${#a[@]}"`,
			"zsh:1: not an identifier: a[]\na=[1 2 3] n=3\n",
		},
		{
			`( u[]=6 ); echo "set=[${u+yes}] u=[${u[@]}]"`,
			"zsh:1: not an identifier: u[]\nset=[] u=[]\n",
		},
		{
			`typeset -A m; m[k]=v; ( m[]=9 ); echo "keys=[${(k)m}] vals=[${(v)m}]"`,
			"zsh:1: not an identifier: m[]\nkeys=[k] vals=[v]\n",
		},
	} {
		out, st := runZsh(t, t.TempDir(), c.src)
		if out != c.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", c.src, out, st, c.want)
		}
	}
}

// The neighbors that must not have moved. Brackets holding *something* are a
// subscript, whatever it comes to, and this shell answers each of them its own
// way — a refusal that reached any of them would be this fix in the wrong
// place.
func TestTheSubscriptsThatAreNotWrittenEmpty(t *testing.T) {
	for _, c := range []struct {
		src, want string
		st        int
	}{
		// A subscript that *arrived* empty from an expansion is the same
		// arithmetic question one text further on, and not this one: the
		// expression reader's sentence, and no name in it.
		{`e=(1 2 3); i=; e[$i]=6`, "zsh:1: bad math expression: empty string\n", 1},
		{`g=(1 2 3); g[1]=6; echo "st=$? g=[${g[@]}]"`, "st=0 g=[6 2 3]\n", 0},
	} {
		out, st := runZsh(t, t.TempDir(), c.src)
		if out != c.want || st != c.st {
			t.Errorf("%s = %q (status %d), want %q at %d", c.src, out, st, c.want, c.st)
		}
	}
	// Quotes between the brackets are a subscript holding the empty string
	// rather than brackets with nothing in them, and this shell refuses it as
	// an *expression*. The sentence is asserted only for what it is not: our
	// wording here is `bad math expression: empty string` where zsh 5.9.2
	// writes ``operand expected at `""'``, which is a divergence of its own
	// and not this one. What matters here is that the empty-brackets refusal
	// does not reach it, and that the array is left alone.
	out, st := runZsh(t, t.TempDir(), `( c=(1 2 3); c[""]=6 ); echo "c=[${c[@]}]"`)
	if strings.Contains(out, "not an identifier") {
		t.Errorf(`c[""]=6 = %q, want the expression refusal and not the empty-brackets one`, out)
	}
	if want := "c=[]\n"; !strings.HasSuffix(out, want) || st != 0 {
		t.Errorf(`c[""]=6 = %q (status %d), want it to end %q at 0`, out, st, want)
	}
}
