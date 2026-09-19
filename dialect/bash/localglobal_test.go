// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// The `-g` letter on `local` rather than on `declare`, which is this shell's
// letter on this shell's builtin: the other shell in the panel with a `local`
// that reads letters at all has no `g` among them, so there is nothing to
// disagree with and the reading is the core's.
//
// Measured 2026-09-19 against bash 5.3.20 at /opt/homebrew, `env -i
// PATH=/usr/bin:/bin LC_ALL=C` with a scratch HOME, under `-c`, a script file
// and standard input alike. Eleven shapes written twice — once with `local
// -g` and once with the same line spelled `declare -g` — are the same bytes
// and the same status in every one: past a local, valueless, `+=`, an array
// literal, `-x`, `-r`, `-i`, `-A`, under a call's assignment prefix, from a
// nested call, and through `command`. That is what says the word adds nothing
// of its own once the letter is written, and it is why the engine hands the
// letter to the declaration rather than reading it twice.

// The shape the letter exists for: a local of the same name standing in front
// of it, which this shell writes underneath.
func TestTheGlobalLetterOnLocalWritesPastALocalOfTheSameName(t *testing.T) {
	out, st := runBash(t, t.TempDir(), `q=9
f(){ local q=5; local -g q=1; echo "in[$q]"; }
f
echo "top[$q]"`)
	want := "in[5]\ntop[1]\n"
	if out != want || st != 0 {
		t.Errorf("`local -g` past a local = %q (status %d), want %q", out, st, want)
	}
}

// The same letter over an **array literal**, which is not the scalar store at
// all: the operand is assigned after the builtin has returned, so the letter
// has to reach that assignment too.
func TestTheGlobalLetterOnLocalReachesAnArrayLiteral(t *testing.T) {
	out, st := runBash(t, t.TempDir(), `q=(9 9)
f(){ local q=(5); local -ga q=(1 2); echo "in[${q[*]}]"; }
f
echo "top[${q[*]}]"`)
	want := "in[5]\ntop[1 2]\n"
	if out != want || st != 0 {
		t.Errorf("`local -ga` past a local array = %q (status %d), want %q", out, st, want)
	}
}

// The letter's attributes land on the shell's own cell with the value, which
// is the row that says the delegation carries the whole declaration and not
// the store alone.
func TestTheGlobalLetterOnLocalCarriesTheAttributes(t *testing.T) {
	out, st := runBash(t, t.TempDir(), `f(){ local -gr rr=3; local -gi ii=2+3; local -gA aa=([k]=v); }
f
declare -p rr ii aa`)
	want := "declare -r rr=\"3\"\ndeclare -i ii=\"5\"\ndeclare -A aa=([k]=\"v\" )\n"
	if out != want || st != 0 {
		t.Errorf("`local -g` with attributes = %q (status %d), want %q", out, st, want)
	}
}

// And the letter says nothing about where the word may be written: `local` is
// still refused outside a function, at 1, with nothing declared.
func TestTheGlobalLetterDoesNotMakeLocalLegalOutsideAFunction(t *testing.T) {
	out, st := runBash(t, t.TempDir(), `local -g zz=1
echo "st[$?]"
echo "zz[${zz-UNSET}]"`)
	want := "bash: line 1: local: can only be used in a function\nst[1]\nzz[UNSET]\n"
	if out != want || st != 0 {
		t.Errorf("`local -g` at the top level = %q (status %d), want %q", out, st, want)
	}
}
