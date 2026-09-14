// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
)

// `${(list)}` runs the parenthesized subshell and expands to what it printed.
// One column has it and we refused every spelling of it (#2615).
//
// Measured 2026-09-13 on ksh93u+ 2012-08-01 at /bin/ksh, over `-c` except
// where a row says otherwise.
func TestAParenthesizedSubshellIsASubstitution(t *testing.T) {
	for _, c := range []struct {
		src, want string
		status    int
	}{
		// The four lines the issue was filed from.
		{`echo ${(echo hi)}`, "hi\n", 0},
		{`echo ${( echo hi )}`, "hi\n", 0},
		{`echo A${(echo hi)}B`, "AhiB\n", 0},
		{`echo "A${(echo hi)}B"`, "AhiB\n", 0},
		// What the body is: a subshell, so nothing it assigns survives and
		// nowhere it goes is anywhere the caller went.
		{`v=1; echo ${(v=2; echo x)}; echo "v=$v"`, "x\nv=1\n", 0},
		// Its status is not the word's: `echo` succeeded on an empty
		// expansion, exactly as it does behind `$( )`.
		{`echo "[${(exit 3)}]"; echo "st=$?"`, "[]\nst=0\n", 0},
		// The end is found by reading a list rather than by counting, so a
		// `)` inside quotes closes nothing and a nested one is stepped over.
		{`echo ${(echo ")" )}`, ")\n", 0},
		{`echo ${( (echo a) )}`, "a\n", 0},
		{`echo ${(echo ${(echo a)})}`, "a\n", 0},
		// Two of them in one word, which is what says the scan stops at the
		// brace behind its own parenthesis rather than at the last one.
		{`echo ${(echo a)}${(echo b)}`, "ab\n", 0},
		// Trailing newlines come off as they do from every other spelling.
		{`printf "[%s]\n" "${(printf "a\nb\n")}"`, "[a\nb]\n", 0},
	} {
		out, st := kshOut(t, c.src)
		if out != c.want || st != c.status {
			t.Errorf("%s\n got %q at %d\nwant %q at %d", c.src, out, st, c.want, c.status)
		}
	}
}

// The extent is the parenthesis and not a command list, which is the whole
// reason this is a spelling of its own rather than `${ cmd;}` with a `(` for
// an opener. `ksh -n` refuses each of the first three while *reading*, so the
// shape is settled before anything runs.
//
// Every row here is a refusal in ksh93 and a refusal here, and the row after
// this one is the contrast that says the refusals are about the paren rather
// than about a subshell standing first in a body.
func TestAParenBodyDoesNotRunOnPastItsParenthesis(t *testing.T) {
	for _, src := range []string{
		// A second command after the `)`.
		`echo ${(echo a); echo b;}`,
		// Only a terminator after it.
		`echo ${(echo a);}`,
		// Only a blank. Nothing at all may stand between the `)` and the `}`.
		`echo ${(echo a) ;}`,
		// A word tail, which is the shape every zsh flag group reaching this
		// parser has. ksh93 blames `b}` and so do we.
		`echo ${(echo a)b}`,
	} {
		out, st := kshOut(t, src)
		if st == 0 {
			t.Errorf("%s\n ran, at status 0, output %q — ksh93 refuses it", src, out)
		}
	}
}

// The blank-opened body *is* a list, and the same two commands behind a blank
// run. Without this row the four refusals above would read as "a subshell may
// not open a body", which is not what was measured.
func TestABlankOpenedBodyHoldingASubshellIsAList(t *testing.T) {
	out, st := kshOut(t, `echo ${ (echo a); echo b;}`)
	if out != "a b\n" || st != 0 {
		t.Errorf("got %q at %d, want %q at 0", out, st, "a b\n")
	}
}

// The word tails this parser meets in the wild are zsh expansion flags, and
// they keep the diagnostic #2374 spells for them: the paren rule declines the
// word and the parameter form reads it, which is ksh93's own order — its
// `-n` passes `${(U)a}` and refuses `${(echo a);}`.
//
// The blamed token runs *past* the `}` in each, which is the property a
// reading that cut the body at the matching brace could not produce.
func TestAFlagGroupStillNamesTheWordTail(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`echo ${(U)a}`, "syntax error at line 1: `a}' unexpected"},
		{`echo "AA${(U)a}BB"`, "syntax error at line 1: `a}BB' unexpected"},
		{`echo ${(a; b)x}`, "syntax error at line 1: `x}' unexpected"},
		{`echo ${(q-)v}`, "syntax error at line 1: `v}' unexpected"},
		{`echo ${(echo a)-b}`, "syntax error at line 1: `-b}' unexpected"},
		// The two adjacent parens are ksh93's braced arithmetic, `${((1+2))}`
		// being 3 there. Not implemented, and deliberately left out of the
		// paren rule so that reading it as a subshell does not answer
		// nothing where that shell answers a number — so it is still the
		// refusal it was, which is what this row holds down.
		{`echo ${((1+2))}`, "syntax error at line 1: `(' unexpected"},
	} {
		if got := refusal(t, c.src); got != c.want {
			t.Errorf("%q:\n got %q\nwant %q", c.src, got, c.want)
		}
	}
}

// And the flag does not leak. The other five columns call `${(echo hi)}` a
// bad substitution — bash 5.3.15, that binary as `sh`, bash 3.2.57, dash and
// BusyBox ash — and zsh reads the parenthesis as its own expansion flags and
// complains about the letters inside, which is a reading of its own and not
// this construct.
func TestAParenSubstitutionIsKshsAlone(t *testing.T) {
	for _, d := range []struct {
		name string
		on   bool
	}{
		{"ksh", ksh.Dialect().SubshellSubstitution},
		{"bash", bash.Dialect().SubshellSubstitution},
		{"zsh", zsh.Dialect().SubshellSubstitution},
		{"dash", dash.Dialect().SubshellSubstitution},
	} {
		if d.name == "ksh" && !d.on {
			t.Errorf("ksh: SubshellSubstitution is off, want it on")
		}
		if d.name != "ksh" && d.on {
			t.Errorf("%s: SubshellSubstitution is on, want it off", d.name)
		}
	}
}
