// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// runDupWidth runs src with the axis answered the given way and a wording of
// its own, so what a test sees is the axis and not the setup.
func runDupWidth(t *testing.T, src string, refuses Answer) (out string, status int) {
	t.Helper()
	return run(t, src, func(r *Runner) {
		s := permissive()
		s.MultiDigitDuplicationTargetIsAnError = refuses
		r.Semantics = &s
		r.Diagnostics = &Diagnostics{MultiDigitDuplicationTarget: "refused: too wide"}
	})
}

// A duplication target written with more than one digit is refused outright
// where the axis says so, and read as a number everywhere else.
//
// Asserted on what ran afterwards as well as on the wording: the refusal ends
// the script, and a shell that complained and carried on would otherwise look
// the same in the first assertion alone.
func TestAWideDuplicationTargetIsRefusedWhereTheAxisSaysSo(t *testing.T) {
	out, st := runDupWidth(t, "echo hi >&10\necho after\n", Yes)
	if !strings.Contains(out, "refused: too wide") {
		t.Errorf("got %q, want the dialect's wording", out)
	}
	if strings.Contains(out, "after") {
		t.Errorf("got %q, want the script to have stopped", out)
	}
	if st != 2 {
		t.Errorf("status = %d, want this vector's fatal status", st)
	}

	out, st = runDupWidth(t, "echo hi >&10\necho after\n", No)
	if strings.Contains(out, "refused: too wide") {
		t.Errorf("got %q, want no refusal where the axis says the number is read", out)
	}
	if !strings.Contains(out, "Bad file descriptor") {
		t.Errorf("got %q, want the descriptor's own failure", out)
	}
	if !strings.Contains(out, "after") || st != 0 {
		t.Errorf("got %q status %d, want the script to have carried on", out, st)
	}
}

// It is the width and not the value. `08` names descriptor 8 — a number every
// answer would otherwise accept — and it is refused for having two digits,
// which is what keeps this separate from the axis about numbers the open-file
// limit will not give out.
func TestTheRefusalIsAboutTheWidthAndNotTheValue(t *testing.T) {
	if out, _ := runDupWidth(t, "echo hi >&08\n", Yes); !strings.Contains(out, "refused: too wide") {
		t.Errorf("got %q, want a two-digit 8 refused", out)
	}
	// And one digit is never asked about, whatever it names: `>&9` is not
	// open here and fails as a descriptor rather than as a word.
	out, _ := runDupWidth(t, "echo hi >&9\necho after\n", Yes)
	if strings.Contains(out, "refused: too wide") {
		t.Errorf("got %q, want a single digit past the width question", out)
	}
	if !strings.Contains(out, "after") {
		t.Errorf("got %q, want the script to have carried on", out)
	}
}

// The check is on the expanded word rather than on what was typed, which is
// what says it cannot belong to the lexer: the same source is refused or not
// depending on what a variable holds when the redirection is applied.
func TestTheWidthIsMeasuredAfterTheTargetExpands(t *testing.T) {
	out, _ := runDupWidth(t, "n=10\necho hi >&$n\necho after\n", Yes)
	if !strings.Contains(out, "refused: too wide") || strings.Contains(out, "after") {
		t.Errorf("got %q, want the expanded word refused", out)
	}

	out, _ = runDupWidth(t, "n=9\necho hi >&$n\necho after\n", Yes)
	if strings.Contains(out, "refused: too wide") {
		t.Errorf("got %q, want one digit taken however it arrived", out)
	}
	if !strings.Contains(out, "after") {
		t.Errorf("got %q, want the script to have carried on", out)
	}
}

// Closing is not a target, so `>&-` is never asked about — nor is a word that
// is not a number at all, which is a different question with a different
// answer in the dialects that have `&>`.
func TestClosingAndNonNumbersAreNotAskedAbout(t *testing.T) {
	if out, st := runDupWidth(t, "exec 3>&-\necho after\n", Yes); !strings.Contains(out, "after") || st != 0 {
		t.Errorf("got %q status %d, want a close to be a close", out, st)
	}
	if out, _ := runDupWidth(t, "echo hi >&xy\n", Yes); strings.Contains(out, "refused: too wide") {
		t.Errorf("got %q, want a word that is not a number left to its own answer", out)
	}
}

// Reading is refused the same way writing is: the rule is about the width of
// the word, not the direction of the operator.
func TestTheRefusalCoversBothDirections(t *testing.T) {
	for _, src := range []string{"echo hi >&10\necho after\n", "cat <&10\necho after\n"} {
		out, _ := runDupWidth(t, src, Yes)
		if !strings.Contains(out, "refused: too wide") || strings.Contains(out, "after") {
			t.Errorf("%q gave %q, want the same refusal", src, out)
		}
	}
}

// With no answer the shell refuses rather than guessing, and names the axis.
func TestWithNoAnswerTheWidthAxisIsRefusedByName(t *testing.T) {
	out, _ := runDupWidth(t, "echo hi >&10\n", Unspecified)
	if !strings.Contains(out, "a duplication target of more than one digit") {
		t.Errorf("got %q, want the axis named in the refusal", out)
	}
}
