// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A single character given to `[[ -o ]]` is the option **letter** it
// abbreviates, in the dialect that reads one.
//
// **Status alone cannot show this.** A letter that names nothing and a letter
// naming an option that is *off* both answer 1, so a grid with every option
// left off agrees with the refusing reading on almost every row — and that is
// not hypothetical: the first version of this change read the letter for
// every dialect, and a hundred-row grid said bash was unmoved. The
// discriminating shape is to turn the option **on** and ask again (#4436).
func TestAConditionOptionMayBeALetter(t *testing.T) {
	withLetters := func(yes bool) func(*Runner) {
		return func(r *Runner) {
			if yes {
				r.Semantics.ConditionOptionTakesAnOptionLetter = Yes
			} else {
				r.Semantics.ConditionOptionTakesAnOptionLetter = No
			}
			r.Semantics.UnknownConditionOptionIsAStatus = No
		}
	}
	// The discriminating pair: the same letter, the option off and then on.
	// Only the second row can tell the two readings apart.
	out, _ := run(t, `[[ -o a ]]; echo "off st=$?"`, withLetters(true))
	if !strings.Contains(out, "off st=1") {
		t.Errorf("letter with the option off = %q, want st=1", out)
	}
	out, _ = run(t, `set -a; [[ -o a ]]; echo "on st=$?"`, withLetters(true))
	if !strings.Contains(out, "on st=0") {
		t.Errorf("letter with the option on = %q, want st=0 — the letter names the option", out)
	}
	// And the same row with the dialect answering no, which is what the
	// other three columns do: the letter is a name this shell has never
	// heard of, so it stays false however the option is set.
	out, _ = run(t, `set -a; [[ -o a ]]; echo "on st=$?"`, withLetters(false))
	if !strings.Contains(out, "on st=1") {
		t.Errorf("answering no = %q, want st=1 — the letter is not read", out)
	}
	// A name still wins, and a multi-character word is never a letter.
	out, _ = run(t, `set -a; [[ -o allexport ]]; echo "name st=$?"`, withLetters(true))
	if !strings.Contains(out, "name st=0") {
		t.Errorf("the name = %q, want st=0", out)
	}
	out, _ = run(t, `[[ -o aa ]]; echo "two st=$?"`, withLetters(true))
	if !strings.Contains(out, "two st=1") {
		t.Errorf("two characters = %q, want it not read as a letter", out)
	}
}
