// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `unset` of one of this shell's produced tables takes the **parameter** away
// and leaves the thing it was a view of standing — #4764.
//
// Measured 2026-09-27 on `/opt/homebrew/bin/zsh`, zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `-f` from a script file under `env -i
// PATH=/usr/bin:/bin`; `go version -m` says *not a Go executable* for it and
// `github.com/blairham/sh/cmd/zsh` for ours.
//
//	alias ex=echo; unset aliases
//	alias ex                    ex=echo — the alias is still there
//	alias foo=bar; alias foo    foo=bar — a new one still lands
//	$aliases[foo]               empty
//	${(t)aliases}               empty
//	aliases=str; print $aliases str, an ordinary scalar
//
// The alias rows are the whole of why this is not the produced *array*'s
// answer, where `unset argv` is delivered as a write of no elements: an array
// producer is the thing, and a table producer here is a view of something the
// shell keeps somewhere else.
//
// The `${(t)}` row is what made it worth its own issue rather than a
// footnote: it already answered `empty` before this, so the name's reading of
// itself and the view behind it disagreed.
func TestUnsettingAProducedTableLeavesTheAliasesStanding(t *testing.T) {
	const src = `alias ex=echo
unset aliases
alias ex
alias foo=bar
alias foo
print -r -- "param=[$aliases[foo]] t=[${(t)aliases}] keys=[${(k)aliases}]"`
	const want = "ex=echo\nfoo=bar\nparam=[] t=[] keys=[]\n"
	out, st := runZsh(t, t.TempDir(), src)
	if out != want || st != 0 {
		t.Errorf("= %q (status %d), want %q", out, st, want)
	}
}

// The same for the function table, which is the other view a script is likely
// to unset by accident, and the row that says this is about produced tables
// rather than about aliases.
func TestUnsettingTheFunctionTableLeavesTheFunctionsCallable(t *testing.T) {
	const src = `g() { print inG; }
unset functions
g
h() { print inH; }
h
print -r -- "t=[${(t)functions}] v=[$functions[h]]"`
	const want = "inG\ninH\nt=[] v=[]\n"
	out, st := runZsh(t, t.TempDir(), src)
	if out != want || st != 0 {
		t.Errorf("= %q (status %d), want %q", out, st, want)
	}
}

// And the name is an ordinary one afterwards: a scalar stored under it reads
// back as itself rather than as the view, and the type word says `scalar`.
//
// This is the store half of #4764, and the row #4639's refusal correctly does
// not reach — a name the script has taken away is not holding a table.
func TestAStoreOverAnUnsetProducedTableIsAnOrdinaryScalar(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"aliases",
			`unset aliases; aliases=string; print -r -- "v=[$aliases] t=[${(t)aliases}]"`,
			"v=[string] t=[scalar]\n",
		},
		{
			"functions",
			`unset functions; functions=str; print -r -- "v=[$functions] t=[${(t)functions}]"`,
			"v=[str] t=[scalar]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// The control the three above lean on: while the parameter is there it
// answers, and a scalar store over it is refused. Without this a producer
// that had never been registered would pass every row.
func TestTheProducedTableAnswersUntilItIsUnset(t *testing.T) {
	const src = `alias ex=echo
print -r -- "param=[$aliases[ex]] assoc=${${(t)aliases}%%-*}"
aliases=string
print -r -- "store=$?"`
	out, st := runZsh(t, t.TempDir(), src)
	// The key answers, and the type word says the name is a table — its
	// *letters* are a separate row and are deliberately not asserted here.
	const wantPrefix = "param=[echo] assoc=association\n"
	if !strings.HasPrefix(out, wantPrefix) {
		t.Errorf("= %q, want it to begin %q", out, wantPrefix)
	}
	if !strings.Contains(out, producedTableRefusal) {
		t.Errorf("= %q, want the produced-table refusal in it", out)
	}
	if st == 0 {
		t.Errorf("status 0, want the store to have been refused")
	}
}
