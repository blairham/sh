// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A bare `set -o` does not always take the word behind it. In the three bash
// columns and in ksh93 a word that begins with `-` or `+` is not a name at
// all: the `-o` is a bare one, so it lists, and the word is read as option
// letters of its own. dash, BusyBox ash and zsh take it whatever it looks
// like and refuse it as a name.
//
// Ours took it in every dialect, so `set -o -e` — the shape #2671 measured —
// left errexit **off** here and turns it **on** in bash. The axis is
// Semantics.SetODeclinesADashWord, and every test below answers it in both
// directions from the same script: a run that only ever answers Yes passes
// under an implementation that ignores the field entirely.

// dashWordSem is a vector with the two neighbouring `set` axes pinned, so
// that what moves in these tests is only the one under test. Welding is off,
// which is the reading the three bash columns and the two ash-family columns
// share, and the validating pass is off except where a test turns it on.
func dashWordSem(declines Answer) Semantics {
	sem := PosixSemantics()
	sem.SetOLetterAttachesItsName = No
	sem.SetValidatesOptionLettersFirst = No
	sem.SetODeclinesADashWord = declines
	// Neither refusal ends the script, so a run that declines nothing lives
	// long enough to be asked what it applied. Both are the axes #2629 split
	// apart; pinning them here keeps a refusal's *fatality* out of what these
	// tests read.
	sem.BadSetOptionNameFatal = No
	sem.BadSetOptionLetterFatal = No
	return sem
}

func runDashWord(t *testing.T, src string, declines Answer) (string, int) {
	t.Helper()
	sem := dashWordSem(declines)
	return run(t, src, func(r *Runner) { r.Semantics = &sem })
}

// TestABareSetODeclinesADashWord is the row the issue is named for, read both
// ways from one script.
func TestABareSetODeclinesADashWord(t *testing.T) {
	const src = "set -o -e >/dev/null\necho \"st=$?\"\ncase $- in *e*) echo e=on;; *) echo e=off;; esac\n"

	out, st := runDashWord(t, src, Yes)
	if out != "st=0\ne=on\n" || st != 0 {
		t.Errorf("declining: got %q at %d, want %q — the `-e` is not a name, it is the letter that turns errexit on", out, st, "st=0\ne=on\n")
	}

	out, st = runDashWord(t, src, No)
	if !strings.Contains(out, "set: -e: invalid option name") || !strings.Contains(out, "e=off") {
		t.Errorf("taking it: got %q at %d, want `-e` refused as an option name with errexit still off", out, st)
	}
}

// The listing is not incidental: a declined word leaves a *bare* `-o`, and a
// bare `-o` writes the whole option table before the loop reads on. A
// dialect that quietly skipped the word instead would pass the test above and
// fail this one.
func TestADeclinedWordLeavesABareSetOThatLists(t *testing.T) {
	out, st := runDashWord(t, "set -o -e\necho done\n", Yes)
	if st != 0 || !strings.Contains(out, "errexit") || !strings.HasSuffix(out, "done\n") {
		t.Errorf("got %q at %d, want the option table on standard output and then `done`", out, st)
	}
}

// A name-shaped word is still the name, which is the other half of the same
// rule: the axis is asked about the spelling of the word and not about the
// `-o`, so `set -o errexit` reads alike under both answers.
func TestANameShapedWordIsStillTheName(t *testing.T) {
	const src = "set -o errexit\necho \"st=$?\"\ncase $- in *e*) echo e=on;; *) echo e=off;; esac\n"
	for _, a := range []Answer{Yes, No} {
		out, st := runDashWord(t, src, a)
		if out != "st=0\ne=on\n" || st != 0 {
			t.Errorf("%v: got %q at %d, want the name taken and errexit on", a, out, st)
		}
	}
}

// The empty word rides with the dash words. It is not a name in the columns
// that decline — `set -o ""` lists there and leaves the empty word as the
// first operand — and it is refused as one in the columns that do not.
func TestABareSetODeclinesTheEmptyWord(t *testing.T) {
	const src = "set -o \"\" >/dev/null\necho \"st=$? n=$# 1=[$1]\"\n"

	out, st := runDashWord(t, src, Yes)
	if out != "st=0 n=1 1=[]\n" || st != 0 {
		t.Errorf("declining: got %q at %d, want the listing and the empty word left as $1", out, st)
	}

	out, st = runDashWord(t, src, No)
	if !strings.Contains(out, "set: : invalid option name") || strings.Contains(out, "n=1") {
		t.Errorf("taking it: got %q at %d, want the empty word refused as an option name", out, st)
	}
}

// Where the dialect also reads every option word's letters before applying
// one, a declined word is a word that pass can now see — which is why bash's
// `set -o -Z` refuses `Z` and writes **no** listing at all. The two axes meet
// nowhere else, and with the decline off the same script reaches the name
// table instead.
func TestADeclinedWordIsSeenByTheValidatingPass(t *testing.T) {
	const src = "set -o -Z\necho after\n"

	sem := dashWordSem(Yes)
	sem.SetValidatesOptionLettersFirst = Yes
	out, st := run(t, src, func(r *Runner) { r.Semantics = &sem })
	if strings.Contains(out, "errexit") || !strings.Contains(out, "set: -Z: invalid option\n") ||
		!strings.HasSuffix(out, "after\n") {
		t.Errorf("got %q at %d, want the letter refused with no option table written", out, st)
	}

	sem = dashWordSem(No)
	sem.SetValidatesOptionLettersFirst = Yes
	out, st = run(t, src, func(r *Runner) { r.Semantics = &sem })
	if strings.Contains(out, "errexit") || !strings.Contains(out, "set: -Z: invalid option name") {
		t.Errorf("not declining: got %q at %d, want `-Z` refused as an option name", out, st)
	}
}

// Unanswered refuses by name rather than guessing, and only where there is a
// word to decline: a dialect that never writes the field still reads
// `set -o errexit` and a bare `set -o`.
func TestAnUnansweredDeclineIsAskedOnlyWhereItMatters(t *testing.T) {
	out, st := runDashWord(t, "set -o -e\necho after\n", Unspecified)
	if !strings.Contains(out, "SetODeclinesADashWord") {
		t.Errorf("got %q at %d, want the axis refused by name", out, st)
	}

	out, st = runDashWord(t, "set -o errexit\necho after\n", Unspecified)
	if out != "after\n" || st != 0 {
		t.Errorf("a name-shaped word: got %q at %d, want the axis never consulted", out, st)
	}

	out, st = runDashWord(t, "set -o >/dev/null\necho after\n", Unspecified)
	if out != "after\n" || st != 0 {
		t.Errorf("a bare `-o` at the end: got %q at %d, want the axis never consulted", out, st)
	}
}

// The predicate is the loop's own reading of a word, and a one-character `-`
// is not one it reads as options — `set - a b` leaves three positional
// parameters here where bash and ksh93 leave two. So `set -o -` is still a
// name, which is measured-wrong in both columns that decline and is pinned
// here rather than papered over: the day the bare `-` is read as an option
// word, this follows it and this test is what says so.
func TestABareDashIsNotYetADeclinedWord(t *testing.T) {
	out, st := runDashWord(t, "set -o -\necho after\n", Yes)
	if strings.Contains(out, "errexit") || !strings.Contains(out, "set: -: invalid option name") {
		t.Errorf("got %q at %d, want `-` still taken as the option name — see the corners on Semantics.SetODeclinesADashWord", out, st)
	}
}
