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

// dashWordSem is a vector with the two neighboring `set` axes pinned, so
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
	// And the listing these tests read is the immediate one, which is the
	// five-column answer: a declined word is where the deferred reading and
	// this one part company, so leaving it unanswered would refuse every row
	// here by name rather than list. See SetListsOptionsOnceAtTheEnd, which
	// has a pair of tests of its own under dialect/.
	sem.SetListsOptionsOnceAtTheEnd = No
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
	if !strings.Contains(out, "declining a next word that begins with a dash") ||
		!strings.Contains(out, "no dialect was chosen") {
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

// The predicate is the loop's own reading of a word, and since #2699 a
// one-character `-` **is** one it reads as options. So `set -o -` declines the
// `-` and lists, which is what both columns that decline a dash word do.
//
// This test was written the other way round — pinning the gap, and saying that
// the day the bare `-` became an option word it would follow and this test
// would be what said so. It said so: closing #2699 reddened it, and this is
// the same assertion with the answer moved.
func TestABareDashIsADeclinedWord(t *testing.T) {
	out, st := runDashWord(t, "set -o -\necho after\n", Yes)
	if strings.Contains(out, "invalid option name") || !strings.Contains(out, "errexit") {
		t.Errorf("got %q at %d, want the `-` declined and the table listed", out, st)
	}
	if !strings.HasSuffix(out, "after\n") {
		t.Errorf("got %q, want the script to carry on after the listing", out)
	}
	// And the other half of the same predicate: the word is consumed rather
	// than becoming an operand. `set - a b` leaves two positional parameters
	// in every column of the panel, which is unanimous and is why it is not
	// an axis.
	out, st = runDashWord(t, `set - a b; echo "n=$# 1=[$1]"`+"\n", Yes)
	if out != "n=2 1=[a]\n" || st != 0 {
		t.Errorf("got %q at %d, want n=2 1=[a]", out, st)
	}
}

// What a bare `-` or `+` *means* once it is consumed, which is a three-way and
// not a bool — the reason [BareOptionWordReading] is an enum.
//
// Consuming the word is unanimous across the panel and is not asked here.
// Clearing `-x` and `-v` is not: `-` clears in four columns and `+` clears in
// one, so no single flag on the dash holds both readings. Measured 2026-09-13
// over script files under `env -i PATH=/usr/bin:/bin` (#2699):
//
//	                       set -v -      set -v +
//	four of the columns    verbose off   verbose on
//	one column             verbose off   verbose off
//	one column             verbose on    verbose on
//
// `set +v -` leaves verbose off under every reading, which is what says the
// rule clears rather than toggles — a toggle would turn it back on.
func TestABareOptionWordMeansThreeDifferentThings(t *testing.T) {
	for _, tc := range []struct {
		name             string
		reading          BareOptionWordReading
		afterDash        string
		afterPlus        string
		afterDashWhenOff string
	}{
		{"inert", BareOptionWordIsInert, "on", "on", "off"},
		{"the dash alone", BareDashClearsTraceAndVerbose, "off", "on", "off"},
		{"either sign", BareEitherSignClearsTraceAndVerbose, "off", "off", "off"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := dashWordSem(Yes)
			sem.BareOptionWord = tc.reading
			probe := func(src string) string {
				out, _ := run(t, src+"\ncase $- in *v*) echo on;; *) echo off;; esac\n",
					func(r *Runner) { r.Semantics = &sem })
				return strings.TrimSpace(out)
			}
			if got := probe("set -v -"); got != tc.afterDash {
				t.Errorf("`set -v -` = %s, want %s", got, tc.afterDash)
			}
			if got := probe("set -v +"); got != tc.afterPlus {
				t.Errorf("`set -v +` = %s, want %s", got, tc.afterPlus)
			}
			// The control: it clears rather than toggles, so a word arriving
			// where the option is already off leaves it off under all three.
			if got := probe("set +v -"); got != tc.afterDashWhenOff {
				t.Errorf("`set +v -` = %s, want %s", got, tc.afterDashWhenOff)
			}
			// And the consumption, which no reading changes.
			out, _ := run(t, `set - a b; echo "n=$#"`+"\n", func(r *Runner) { r.Semantics = &sem })
			if out != "n=2\n" {
				t.Errorf("`set - a b` = %q, want n=2 under every reading", out)
			}
		})
	}
}

// A bare `-` is also where the option parse *stops*, which is the half the
// consumption row above cannot ask.
//
// `set - a b` breaks the loop on `a` whatever the `-` did, so a reading that
// carries on past the word and one that stops at it score alike there. `set
// -e - -Z` tells them apart: carrying on reads `-Z` as option letters and
// refuses it, stopping makes it the one positional parameter. Measured
// 2026-09-13 from a script file under `env -i` — all seven columns are
// errexit **on**, silent about `-Z`, and `$1` is `-Z`. Unanimous, so it is
// asked of no dialect and holds under every reading of what the word *means*.
func TestABareOptionWordEndsTheOptionParse(t *testing.T) {
	for _, tc := range []struct {
		name    string
		reading BareOptionWordReading
	}{
		{"inert", BareOptionWordIsInert},
		{"the dash alone", BareDashClearsTraceAndVerbose},
		{"either sign", BareEitherSignClearsTraceAndVerbose},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := dashWordSem(Yes)
			sem.BareOptionWord = tc.reading
			probe := func(src string) (string, int) {
				return run(t, src, func(r *Runner) { r.Semantics = &sem })
			}

			out, st := probe("set -e - -Z\n" +
				"case $- in *e*) echo e=on;; *) echo e=off;; esac\n" +
				`echo "n=$# 1=[$1]"` + "\n")
			if out != "e=on\nn=1 1=[-Z]\n" || st != 0 {
				t.Errorf("`set -e - -Z` gave %q at %d, want errexit on with `-Z` as the one parameter", out, st)
			}

			// And the spelling that takes a word of its own: an `-o` behind
			// the `-` is not an `-o`, it is a parameter, and so is the name
			// that would have followed it.
			out, st = probe("set -u - -o zzznosuch\n" +
				"case $- in *u*) echo u=on;; *) echo u=off;; esac\n" +
				`echo "n=$# 1=[$1] 2=[$2]"` + "\n")
			if out != "u=on\nn=2 1=[-o] 2=[zzznosuch]\n" || st != 0 {
				t.Errorf("`set -u - -o zzznosuch` gave %q at %d, want nounset on with two parameters", out, st)
			}

			// A `--` behind the word is a parameter too, which is what says
			// the parse really ended rather than merely skipping a word: a
			// loop still reading options would have taken it as the marker.
			out, st = probe("set -e - -- x\n" + `echo "n=$# 1=[$1] 2=[$2]"` + "\n")
			if out != "n=2 1=[--] 2=[x]\n" || st != 0 {
				t.Errorf("`set -e - -- x` gave %q at %d, want `--` kept as a parameter", out, st)
			}
		})
	}
}
