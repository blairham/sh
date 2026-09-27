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

// The grid #4811 asked for: `unset` against every produced parameter this
// shell has, in both shells, in one run — and the row that issue was filed
// for did not survive it.
//
// The claim was that `unset parameters` is refused here and taken in the
// reference, with the name going. It is taken in a *fresh* reference, and the
// reason is not this question at all: `zsh/parameter` is not loaded in a
// `-f` shell, so `$parameters` is still zsh's **autoload stub** for the
// module — an ordinary scalar holding the string `zsh/parameter` — and the
// unset removes the stub. Every access but `unset` materializes the parameter
// first, which is the same mechanism the note at the top of dialect/zsh's
// parameter.go records for `unset "functions[m]"` (#1527).
//
// Measured 2026-09-27 on `/opt/homebrew/bin/zsh`, `zsh 5.9.2
// (aarch64-apple-darwin25.4.0)`, `-f` from a script file under `env -i
// PATH=/usr/bin:/bin`, with `zmodload -e zsh/parameter` beside each step:
//
//	fresh shell                             not loaded
//	unset parameters                        status 0, still not loaded
//	${(t)parameters} after that unset       empty, still not loaded
//	${(t)parameters} first                  association-readonly-hide-hideval-special,
//	                                        and loaded afterwards
//	unset parameters after that read        read-only variable: parameters
//
// That reading of the mechanism was right and the conclusion drawn from it —
// that the refusal is the reference's answer for every state *this* shell can
// be in, because it produces the table from the start and has no module to
// autoload — stopped being true when the shell learned the state (#4895).
// A deferred parameter is now exactly that stub: nothing has referred to the
// name, so there is no parameter for the freeze to be on and the `unset`
// takes the name away. See interp/deferredparam.go.
//
// So the grid below is run **twice**, and the two halves are the measurement
// rather than a belt-and-braces: with nothing in front of it every name is a
// silent 0 in both shells, and with a `${+name}` in front the refused column
// is the refusal in both. A grid run only the first way would pass on a shell
// that had stopped freezing anything, and one run only the second way is the
// grid that hid the mechanism for two issues running.
func TestUnsetAcrossEveryProducedParameter(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name    string
		refused bool
		// ours marks a parameter this shell registers that a bare reference
		// has not got at all: `zsh/system` and `zsh/datetime` declare no
		// autoloadable parameter, so `typeset -p sysparams` is `no such
		// variable` at 1 there where every deferred name is nothing at 0.
		// They are therefore deliberately *not* deferred — the gap is that
		// this shell has them before a `zmodload`, which is a different
		// question — and the row asserts this shell's own freeze rather than
		// a comparison. Measured 2026-09-27 on zsh 5.9.2.
		ours bool
	}{
		{"commands", false, false},
		{"options", false, false},
		{"aliases", false, false},
		{"galiases", false, false},
		{"saliases", false, false},
		{"functions", false, false},
		{"dis_aliases", false, false},
		{"dis_galiases", false, false},
		{"dis_saliases", false, false},
		{"dis_functions", false, false},
		{"nameddirs", false, false},
		{"mapfile", false, false},
		{"dirstack", false, false},
		{"parameters", true, false},
		{"builtins", true, false},
		{"history", true, false},
		{"dis_functions_source", true, false},
		{"jobdirs", true, false},
		{"jobtexts", true, false},
		{"jobstates", true, false},
		{"sysparams", true, true},
		{"reswords", true, false},
		{"dis_reswords", true, false},
		{"dis_patchars", true, false},
		{"funcstack", true, false},
		{"functrace", true, false},
		{"funcsourcetrace", true, false},
		{"funcfiletrace", true, false},
		{"errnos", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Nothing has referred to the name, so there is no parameter to
			// refuse the removal: silent 0 for every row, refused column
			// included.
			out, st := runZsh(t, dir, "unset "+tc.name+`; print "st=$?"`)
			want, wantStatus := "st=0\n", 0
			if tc.ours && tc.refused {
				want, wantStatus = "zsh:1: read-only variable: "+tc.name+"\n", 1
			}
			if out != want || st != wantStatus {
				t.Errorf("unset %s with nothing in front = %q (status %d), want %q at %d",
					tc.name, out, st, want, wantStatus)
			}
			// And with the lightest reference there is in front of it, the
			// freeze is there and the column splits.
			out, st = runZsh(t, dir, ": ${+"+tc.name+"}\nunset "+tc.name+`; print "st=$?"`)
			want, wantStatus = "st=0\n", 0
			if tc.refused {
				want, wantStatus = "zsh:2: read-only variable: "+tc.name+"\n", 1
			}
			if out != want || st != wantStatus {
				t.Errorf("unset %s after a reference = %q (status %d), want %q at status %d",
					tc.name, out, st, want, wantStatus)
			}
		})
	}
}
