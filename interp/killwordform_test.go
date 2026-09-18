// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// How `kill` reads the word in front of its targets, which is three questions
// the panel answers separately: whether `-n` is an option at all, what an
// option with nothing after it is, and what a word written where a number goes
// and that is not one is called.

func wordSem() Semantics {
	s := killSem()
	s.KillReadsTheNumberOption = Yes
	s.KillOptionWithNoArgumentIsASignalName = No
	return s
}

// TestTheNumberOptionIsAnOptionOnlyWhereTheVectorSaysSo pins
// Semantics.KillReadsTheNumberOption at the shape the two columns without it
// produce: the word falls through to the bare `-SPEC` reading, so `n` is the
// spec and the complaint is the one that reading already has.
func TestTheNumberOptionIsAnOptionOnlyWhereTheVectorSaysSo(t *testing.T) {
	dg := Diagnostics{
		KillIllegalOption: "kill: illegal option -%[2]s",
		KillInvalidSignal: "kill: bad signal %[1]s",
	}

	// With the option, the number after it is the signal and the word is
	// gone from the operands.
	_, errs, _ := killRun(t, "kill -n 99 999999\n", wordSem(), dg)
	if !strings.Contains(errs, "bad signal 99") {
		t.Errorf("with the option: stderr %q, want the *number* refused", errs)
	}

	// Without it, `-n` is a dash-word like any other and `n` is the spec.
	noOption := wordSem()
	noOption.KillReadsTheNumberOption = No
	_, errs, _ = killRun(t, "kill -n 99 999999\n", noOption, dg)
	if !strings.Contains(errs, "illegal option -n") {
		t.Errorf("without the option: stderr %q, want the *letter* refused", errs)
	}

	// And a vector that has not answered refuses rather than guessing, which
	// matters because the two answers aim a signal at different things.
	silent := wordSem()
	silent.KillReadsTheNumberOption = Unspecified
	_, errs, st := killRun(t, "kill -n 99 999999\n", silent, dg)
	if st == 0 || !strings.Contains(errs, "no dialect was chosen") {
		t.Errorf("unanswered: status %d, stderr %q, want a refusal", st, errs)
	}
}

// TestAnOptionWithNothingAfterItIsEitherMissingOrASignal pins the other
// reading of the same word.
func TestAnOptionWithNothingAfterItIsEitherMissingOrASignal(t *testing.T) {
	dg := Diagnostics{
		KillMissingSignalArgument: "kill: %[1]s: option requires an argument",
		KillInvalidSignal:         "kill: bad signal %[1]s",
	}

	_, errs, _ := killRun(t, "kill -s\n", wordSem(), dg)
	if !strings.Contains(errs, "-s: option requires an argument") {
		t.Errorf("stderr %q, want the option complaint", errs)
	}

	asSignal := wordSem()
	asSignal.KillOptionWithNoArgumentIsASignalName = Yes
	_, errs, _ = killRun(t, "kill -s\n", asSignal, dg)
	if !strings.Contains(errs, "bad signal s") {
		t.Errorf("stderr %q, want the letter read as the signal it is not", errs)
	}
}

// TestAWordWrittenWhereANumberGoesHasItsOwnWording is #3167's first half.
//
// A word that is number-shaped and is not a number is a third route in one
// column, and every other column keeps the wording the *form* already had —
// which is the whole reason the form travels with the error rather than the
// kind being chosen by whether a dialect has the new string.
func TestAWordWrittenWhereANumberGoesHasItsOwnWording(t *testing.T) {
	own := Diagnostics{
		KillInvalidSignalNumber: "invalid signal number: %[5]s",
		KillIllegalOption:       "unknown signal: %[3]s",
		KillInvalidSignal:       "unknown signal: %[3]s",
		KillUnknownSignalHint:   "type kill -l for a list of signals",
	}
	for _, tc := range []struct{ name, src, want string }{
		// The dash belongs to the form: the flag form's operand is the whole
		// word and `-n`'s is what came after it.
		{"the flag form carries its dash", "kill -9x 999999\n", "invalid signal number: -9x"},
		{"the number option does not", "kill -n 9x 999999\n", "invalid signal number: 9x"},
		// A word starting with a letter is not number-shaped, whichever form
		// it arrived in, so it keeps the unknown-signal wording.
		{"a letter first is not this route", "kill -x9 999999\n", "unknown signal: SIGX9"},
		// And `-s` takes a name, so digits there are already the wrong kind
		// of word rather than a number that failed.
		{"-s takes a name", "kill -s 9x 999999\n", "unknown signal: SIG9X"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, errs, st := killRun(t, tc.src, wordSem(), own)
			if !strings.Contains(errs, tc.want) {
				t.Errorf("stderr %q, want %q in it", errs, tc.want)
			}
			if st == 0 {
				t.Error("status 0, want a refusal")
			}
			// The listing hint follows the two unknown-signal routes and not
			// this one, which is measured and is why it is a separate kind
			// rather than a wording swapped in.
			hinted := strings.Contains(errs, "type kill -l")
			if want := !strings.Contains(errs, "invalid signal number"); hinted != want {
				t.Errorf("stderr %q: hint %v, want %v", errs, hinted, want)
			}
		})
	}

	// With no wording of its own, each form falls back to what it already
	// drew — and the two fallbacks are different, which is the point.
	fell := Diagnostics{
		KillIllegalOption:   "illegal option -%[2]s",
		KillInvalidSignal:   "bad signal %[1]s",
		KillBadOptionStatus: 2,
		KillArgumentStatus:  1,
	}
	_, errs, st := killRun(t, "kill -9x 999999\n", wordSem(), fell)
	if !strings.Contains(errs, "illegal option -9") || st != 2 {
		t.Errorf("flag form fallback: stderr %q at %d, want the option complaint at 2", errs, st)
	}
	_, errs, st = killRun(t, "kill -n 9x 999999\n", wordSem(), fell)
	if !strings.Contains(errs, "bad signal 9x") || st != 1 {
		t.Errorf("number option fallback: stderr %q at %d, want the signal complaint at 1", errs, st)
	}
}

// TestAnUnknownOptionIsReportedPerLetterWhereTheVectorSaysSo is #3167's
// second half.
//
// One column's option parser splits a dash-word into its characters and
// complains about each, then writes its usage block once. Repeats are
// repeated and case is kept, both measured.
func TestAnUnknownOptionIsReportedPerLetterWhereTheVectorSaysSo(t *testing.T) {
	dg := Diagnostics{
		KillIllegalOption:          "kill: -%[1]s: unknown option",
		KillIllegalOptionPerLetter: true,
		KillIllegalOptionUsage:     "Usage: kill [-lL] job ...",
	}
	// The prefix is the runner's own name, which is not what this case is
	// about — so the lines are read with it taken off.
	said := func(sem Semantics, dg Diagnostics, src string) string {
		t.Helper()
		_, errs, _ := killRun(t, src, sem, dg)
		return strings.ReplaceAll(errs, "testsh: ", "")
	}
	for _, tc := range []struct{ src, want string }{
		{"kill -NOPE 999999\n", "kill: -N: unknown option\nkill: -O: unknown option\nkill: -P: unknown option\nkill: -E: unknown option\nUsage: kill [-lL] job ...\n"},
		{"kill -99x 999999\n", "kill: -9: unknown option\nkill: -9: unknown option\nkill: -x: unknown option\nUsage: kill [-lL] job ...\n"},
		{"kill -Q 999999\n", "kill: -Q: unknown option\nUsage: kill [-lL] job ...\n"},
	} {
		if got := said(wordSem(), dg, tc.src); got != tc.want {
			t.Errorf("%q: stderr %q, want %q", tc.src, got, tc.want)
		}
	}

	// Without the flag the word is named once, and the block still follows.
	plain := dg
	plain.KillIllegalOptionPerLetter = false
	if got := said(wordSem(), plain, "kill -NOPE 999999\n"); got != "kill: -NOPE: unknown option\nUsage: kill [-lL] job ...\n" {
		t.Errorf("stderr %q, want the whole word once", got)
	}
}
