// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// A single character given to `[[ -o ]]` is the option **letter** it
// abbreviates here, where the other three columns with the operator read it
// as a name they have never heard of.
//
// Measured 2026-09-28 against /opt/homebrew/bin/zsh — zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `go version -m`: *not a Go executable* — one
// character at a time over the letters and the digits (#4436).
//
// **Status alone cannot show this.** A letter that names nothing and a letter
// naming an option that is *off* both answer 1, so a grid with every option
// left off agrees with the refusing reading on almost every row. The rows
// below that turn the option **on** are the evidence; the rest are there to
// pin the roster.
func TestAConditionOptionMayBeALetter(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// The discriminating pair: same letter, option off then on.
		{`[[ -o a ]]; print "st=$?"`, "st=1"},
		{`setopt allexport; [[ -o a ]]; print "st=$?"`, "st=0"},
		{`setopt allexport; [[ ! -o a ]]; print "st=$?"`, "st=1"},
		// More of the same shape, so one letter's table entry is not the
		// whole of the evidence.
		{`setopt errexit; [[ -o e ]]; print "st=$?"`, "st=0"},
		{`setopt nounset; [[ -o u ]]; print "st=$?"`, "st=0"},
		{`setopt interactivecomments; [[ -o k ]]; print "st=$?"`, "st=0"},
		{`setopt cdablevars; [[ -o T ]]; print "st=$?"`, "st=0"},
		{`setopt nullglob; [[ -o G ]]; print "st=$?"`, "st=0"},
		// A name still wins and still works.
		{`setopt allexport; [[ -o allexport ]]; print "st=$?"`, "st=0"},
		{`[[ -o Err_Exit ]]; print "st=$?"`, "st=1"},
		// **A character, not a number.** The digits that answer are letters
		// in that shell's table rather than indices into it: two digits is
		// a name it has never heard of.
		{`[[ -o 10 ]]; print "st=$?"`, "zsh:1: no such option: 10\nst=3"},
		{`[[ -o 07 ]]; print "st=$?"`, "zsh:1: no such option: 07\nst=3"},
		{`[[ -o 25 ]]; print "st=$?"`, "zsh:1: no such option: 25\nst=3"},
		// A letter this shell records as naming *nothing* is still known,
		// so it answers false rather than complaining. `s` is that letter —
		// the table carries it as taken and moving nothing — and without
		// this row the branch that says so is untested: a mutation removing
		// it killed nothing until this was added.
		{`[[ -o s ]]; print "st=$?"`, "st=1"},
		// And a word that is not a letter is refused exactly as before.
		{`[[ -o aa ]]; print "st=$?"`, "zsh:1: no such option: aa\nst=3"},
		{`[[ -o zzznosuch ]]; print "st=$?"`, "zsh:1: no such option: zzznosuch\nst=3"},
	} {
		out, _ := runZsh(t, t.TempDir(), tc.src)
		if strings.TrimSpace(out) != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
		}
	}
}

// TestTheLettersThisOperatorRefuses: the roster has holes, and they are the
// reference's holes rather than ours — `b c j o z A` are `no such option`
// there exactly as a long name it has never heard of is.
func TestTheLettersThisOperatorRefuses(t *testing.T) {
	for _, c := range []string{"b", "c", "j", "o", "z", "A"} {
		src := "[[ -o " + c + " ]]; print \"st=$?\""
		out, _ := runZsh(t, t.TempDir(), src)
		if !strings.Contains(out, "no such option: "+c) {
			t.Errorf("[[ -o %s ]] = %q, want it refused as the reference refuses it", c, out)
		}
	}
}
