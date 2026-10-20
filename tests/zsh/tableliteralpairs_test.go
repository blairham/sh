// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

const oddPairRefusal = "bad set of key/value pairs for associative array"

// A keyed literal whose bare elements come to an **odd** number of fields is
// refused here, and the refusal ends the shell.
//
// Measured 2026-09-26 from a script file under `zsh -f f.sh`, zsh 5.9.2
// (aarch64-apple-darwin25.4.0):
//
//	typeset -A h=(a 1 b)      `f.sh:1: bad set of key/value pairs for
//	                          associative array`, status 1, nothing after it
//	typeset -A h; h=(a 1 b)   the same, so the declaration is not what decides
//	typeset -A h=(a)          the same, so it is not only a trailing word
//	typeset -A h=(x 1); h+=(a 1 b)   the same, so the append spelling asks too
//	aliases=(a 1 b)           the same, so a produced table asks it too
//
// bash takes the odd list and gives the last field a key with nothing under
// it, which is what makes this an axis rather than a bug. We took it too, and
// invented the key silently (#4594).
func TestAnOddKeyedLiteralIsRefusedHere(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a declaration's operand", "typeset -A h=(a 1 b)\nprintf reached"},
		{"an assignment of its own", "typeset -A h\nh=(a 1 b)\nprintf reached"},
		{"a single word", "typeset -A h=(a)\nprintf reached"},
		{"the append spelling", "typeset -A h=(x 1)\nh+=(a 1 b)\nprintf reached"},
		{"a produced table", "aliases=(a 1 b)\nprintf reached"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if !strings.Contains(out, oddPairRefusal) {
				t.Errorf("= %q, want the unpaired-elements refusal", out)
			}
			if strings.Contains(out, "reached") {
				t.Errorf("= %q, want the shell left before the next command", out)
			}
			if st != 1 {
				t.Errorf("status = %d, want 1", st)
			}
		})
	}
}

// The count is over the fields the elements came to, which is the case a
// script hits: the same one written element is refused or taken according to
// what it expands to.
//
// Measured the same day: with `w=(a 1 b)`, `typeset -A h=($w)` is refused, and
// with `w=(a 1 b 2)` it is a two-element table at status 0.
func TestTheOddCountIsOverWhatTheElementsCameTo(t *testing.T) {
	out, st := answersRun(t, "w=(a 1 b)\ntypeset -A h=($w)\nprintf reached")
	if !strings.Contains(out, oddPairRefusal) || strings.Contains(out, "reached") || st != 1 {
		t.Errorf("three fields from one element = %q status %d, want the refusal at 1", out, st)
	}
	out, st = answersRun(t, "w=(a 1 b 2)\ntypeset -A h=($w)\nprintf '[%s][%s]' \"$?\" \"${#h}\"")
	if want := "[0][2]"; out != want || st != 0 {
		t.Errorf("four fields from one element = %q status %d, want %q at 0", out, st, want)
	}
}

// The controls, each of which this shell takes — and each of which a check
// written on the wrong noun would refuse.
//
// Measured the same day: the even list and the empty literal are status 0, and
// a **repeated key** is legal, the later value winning and the table holding
// one element. A check counting keys rather than fields would see one key from
// two pairs there and refuse a line every column accepts. The plain array is
// the fourth: the same three words over a name that is not a table are three
// elements, so it is the target's kind and not the words.
func TestTheEvenAndEmptyAndRepeatedLiteralsAreTakenHere(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"an even list", "typeset -A h=(a 1 b 2)\nprintf '[%s][%s]' \"$?\" \"${#h}\"", "[0][2]"},
		{"an empty literal", "typeset -A h=()\nprintf '[%s][%s]' \"$?\" \"${#h}\"", "[0][0]"},
		{"a repeated key", "typeset -A h=(a 1 a 2)\nprintf '[%s][%s][%s]' \"$?\" \"${#h}\" \"${h[a]}\"", "[0][1][2]"},
		{"a plain array", "h=(a 1 b)\nprintf '[%s][%s]' \"$?\" \"${#h}\"", "[0][3]"},
		{"an empty word among the fields", "e=\ntypeset -A h=(a 1 $e b 2)\nprintf '[%s][%s]' \"$?\" \"${#h}\"", "[0][2]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("= %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// What the refusal costs, which `eval` is what makes askable: the text it was
// given ends and the file above it runs on, so the table can be read after.
//
// Measured the same day, `zsh -f` over a script file:
//
//	typeset -A h=(x 9); eval "h=(a 1 b)"     `x` still 9, one element
//	eval "typeset -A h=(a 1 b)"; typeset -p h   `typeset -A h=( )`
//
// So nothing is stored and the table is not emptied first — and a name the
// same command declared is left declared, associative and empty rather than
// unset, which is the declaration taking effect where the assignment did not.
func TestARefusedPairingStoresNothingHere(t *testing.T) {
	out, st := answersRun(t,
		"typeset -A h=(x 9)\neval 'h=(a 1 b)'\nprintf '[%s][%s][%s][%s]' \"$?\" \"${#h}\" \"${h[x]}\" \"${h[a]-ABSENT}\"")
	if want := "[1][1][9][ABSENT]"; !strings.HasSuffix(out, want) || st != 0 {
		t.Errorf("= %q status %d, want it to end %q at 0", out, st, want)
	}
	out, st = answersRun(t, "eval 'typeset -A h=(a 1 b)'\nprintf '[%s]' \"${+h}\"\ntypeset -p h")
	if want := "[1]typeset -A h=( )\n"; !strings.HasSuffix(out, want) || st != 0 {
		t.Errorf("a fresh declaration = %q status %d, want it to end %q at 0", out, st, want)
	}
}

// And the refusal reaches out of a function, naming it where a top-level one
// names the file: measured, `f() { typeset -A h=(a 1 b) }; f; print after` is
// `f: bad set of key/value pairs for associative array` with nothing after it.
func TestAnOddKeyedLiteralInAFunctionEndsTheShell(t *testing.T) {
	out, st := answersRun(t, "f() { typeset -A h=(a 1 b) }\nf\nprintf reached")
	if !strings.HasPrefix(out, "f: "+oddPairRefusal) {
		t.Errorf("= %q, want the function's name in front of the refusal", out)
	}
	if strings.Contains(out, "reached") || st != 1 {
		t.Errorf("= %q status %d, want the shell left at 1", out, st)
	}
}

// The answer and the wording, pinned so that no preset here drifts off the
// column they were measured from.
func TestThePairingAnswerIsThisDialects(t *testing.T) {
	if got := zsh.Semantics().BareElementsInATableLiteralMustPairOff; got != interp.Yes {
		t.Errorf("BareElementsInATableLiteralMustPairOff = %v, want Yes", got)
	}
	if got := zsh.Diagnostics().UnpairedTableLiteralElements; got != oddPairRefusal {
		t.Errorf("UnpairedTableLiteralElements = %q, want %q", got, oddPairRefusal)
	}
}
