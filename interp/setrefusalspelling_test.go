// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The two refusals `set` can produce are two questions, and #2629 is the
// column that proved it. BusyBox ash reports a refused `set -o` **name** at 1
// and carries on, and ends the script at 2 for a refused option **letter** —
// so `Semantics.BadSetOptionNameFatal` and
// `Diagnostics.SetInvalidOptionNameStatus`, which answered both spellings,
// could not hold it. Both were measured in #483 across six columns that
// agreed; ash became the seventh afterwards (#2272).
//
// Every test here answers the two axes *differently*, which is the only shape
// that can tell them apart: a run where both hold the same value passes under
// either reading and is evidence of nothing.

// ashLike is the vector and wordings of the one panel shell whose two
// spellings disagree, reduced to the four answers that carry the split.
func ashLike() (Semantics, Diagnostics) {
	sem := PosixSemantics()
	sem.BadSetOptionNameFatal = No
	sem.BadSetOptionLetterFatal = Yes
	sem.FatalErrorStatusIsOne = No
	return sem, Diagnostics{
		Location:                     LocationNone,
		SetInvalidOptionName:         "set: illegal option -o %[1]s",
		SetInvalidOptionLetter:       "set: illegal option %[1]s",
		SetInvalidOptionNameStatus:   1,
		SetInvalidOptionLetterStatus: 2,
	}
}

// TestARefusedNameAndARefusedLetterAreTwoAnswers is the whole of #2629 in one
// script: the shape BusyBox ash writes, which no single pair of fields could
// produce.
func TestARefusedNameAndARefusedLetterAreTwoAnswers(t *testing.T) {
	sem, dg := ashLike()
	out, st := run(t, "set -o nosuchname\necho \"st=$?\"\nset -Z\necho after\n", func(r *Runner) {
		r.Semantics, r.Diagnostics = &sem, &dg
	})
	want := "sh: set: illegal option -o nosuchname\nst=1\nsh: set: illegal option -Z\n"
	if out != want || st != 2 {
		t.Errorf("got %q at %d, want %q at 2 — the name survivable at 1, the letter fatal at 2", out, st, want)
	}
}

// And the mirror, so that neither half of the pair can be the one doing all
// the work: a shell where the *name* is the harsher of the two still gets both
// answers right. No panel member is shaped this way, which is exactly why it
// is asserted — the fields have to carry what they are told rather than one
// shell's arrangement of them.
func TestTheSplitIsNotOneShellsArrangement(t *testing.T) {
	sem, dg := ashLike()
	sem.BadSetOptionNameFatal, sem.BadSetOptionLetterFatal = Yes, No
	dg.SetInvalidOptionNameStatus, dg.SetInvalidOptionLetterStatus = 2, 1
	out, st := run(t, "set -Z\necho \"st=$?\"\nset -o nosuchname\necho after\n", func(r *Runner) {
		r.Semantics, r.Diagnostics = &sem, &dg
	})
	want := "sh: set: illegal option -Z\nst=1\nsh: set: illegal option -o nosuchname\n"
	if out != want || st != 2 {
		t.Errorf("got %q at %d, want %q at 2", out, st, want)
	}
}

// TestAGrantedSetOptionAsksNothing pins where the two axes are consulted, the
// way TestACleanForHeaderAsksNothing does one construct over.
//
// `ask` on an unanswered axis refuses and takes the status with it, so an axis
// read at the top of the builtin would make every `set` in the language fail
// under a vector that has not chosen — `set -e`, `set -o noglob`, `set --`,
// and the `set -x` that half the tests in this package start with. The
// disagreement is about a *refusal*, and there is no refusal here for either
// axis to be about.
func TestAGrantedSetOptionAsksNothing(t *testing.T) {
	for _, c := range []struct{ name, src string }{
		{"a letter", `set -u; echo "ok=$?"`},
		{"a letter turned back off", `set -u; set +u; echo "ok=$?"`},
		{"a bundle of letters", `set -ue; echo "ok=$?"`},
		{"a name", `set -o nounset; echo "ok=$?"`},
		{"a name turned back off", `set -o nounset; set +o nounset; echo "ok=$?"`},
		{"the positional parameters", `set -- a b; echo "ok=$?"`},
		{"the listing", `set -o >/dev/null; echo "ok=$?"`},
	} {
		t.Run(c.name, func(t *testing.T) {
			sem := PosixSemantics()
			sem.BadSetOptionNameFatal = Unspecified
			sem.BadSetOptionLetterFatal = Unspecified
			out, st := run(t, c.src, func(r *Runner) { r.Semantics = &sem })
			if out != "ok=0\n" || st != 0 {
				t.Errorf("got %q at %d, want %q at 0 and nothing asked", out, st, "ok=0\n")
			}
		})
	}
}

// And the other half of the same rule: each spelling's *own* refusal reaches
// its own axis, so an unanswered one is refused by name rather than quietly
// picking a side — and the axis that was not asked stays silent.
func TestEachRefusalReachesItsOwnAxis(t *testing.T) {
	for _, c := range []struct{ name, src, why string }{
		{"a letter", "set -Z\n", "a refused `set` option letter ending the script"},
		{"a letter in a bundle", "set -xZ\n", "a refused `set` option letter ending the script"},
		{"a name", "set -o nosuchname\n", "an unknown `set -o` name ending the script"},
		{"a name turned off", "set +o nosuchname\n", "an unknown `set -o` name ending the script"},
	} {
		t.Run(c.name, func(t *testing.T) {
			// The axis this spelling must *not* reach is answered, so a
			// reading through the wrong field would find an answer and say
			// nothing at all.
			sem := PosixSemantics()
			sem.BadSetOptionNameFatal, sem.BadSetOptionLetterFatal = Yes, Yes
			if strings.Contains(c.why, "letter") {
				sem.BadSetOptionLetterFatal = Unspecified
			} else {
				sem.BadSetOptionNameFatal = Unspecified
			}
			out, _ := run(t, c.src, func(r *Runner) { r.Semantics = &sem })
			if !strings.Contains(out, "the shells disagree here and no dialect was chosen") {
				t.Errorf("got %q, want the unanswered axis to be named", out)
			}
			if !strings.Contains(out, c.why) {
				t.Errorf("got %q, want the refusal to say %q", out, c.why)
			}
		})
	}
}

// The attached form is the probe that says the seam is the **spelling** and
// not the `-o` route, and it belongs in the tree because the reading it rules
// out is the one the substrate held for a year.
//
// `set -ozzznosuch` carries an `-o`, so "the `-o` route is the gentle one"
// predicts the name's answer for it — 1, and the script carrying on. Measured
// 2026-09-13, BusyBox ash stops at 2, which is the letter's answer: it reads
// the word as a bare `-o` followed by the letters of `zzznosuch` and refuses
// the `z`. So what decides is the spelling that was refused.
//
// Where the parse lands is a separate question this does not assert, because
// ours and BusyBox's differ on it: this engine refuses the `-o` itself as an
// attached letter where BusyBox lists the options and then refuses the `z`.
// Both are letters and both take this pair, which is what is pinned here; the
// parse itself is #2640.
func TestTheAttachedFormIsALetterAndNotAName(t *testing.T) {
	sem, dg := ashLike()
	out, st := run(t, "set -ozzznosuch\necho after\n", func(r *Runner) {
		r.Semantics, r.Diagnostics = &sem, &dg
	})
	if strings.Contains(out, "after") || st != 2 {
		t.Errorf("got %q at %d, want the letter's fatal 2 and no `after` — an `-o` in the word is not the name's answer", out, st)
	}
	if !strings.Contains(out, "illegal option ") {
		t.Errorf("got %q, want the letter's wording and not the name's", out)
	}
	if strings.Contains(out, "-o zzznosuch") {
		t.Errorf("got %q, want the word read as letters rather than as a name", out)
	}
}

// `set -A` with no name asks the **letter**'s pair, because `-A` is a letter.
// Nothing measurable rides on it today — ksh93 and zsh are the only shells
// with `set -A` and both answer the two spellings alike — so the assertion is
// that the wiring says which one it is rather than taking whichever field was
// nearest.
func TestSetArrayWithNoNameAsksTheLettersAxis(t *testing.T) {
	sem := PosixSemantics()
	sem.SetArrayLetter = Yes
	sem.SetArrayOptionsContinuePastTheName = No
	sem.SetArrayWithNoValuesUnsetsTheName = No
	sem.ArrayBaseIsZero = Yes
	sem.FatalErrorStatusIsOne = No
	sem.BadSetOptionNameFatal = No
	sem.BadSetOptionLetterFatal = Yes
	dg := Diagnostics{
		Location:                     LocationNone,
		SetArrayNeedsAName:           "set: %[1]s: name argument expected",
		SetInvalidOptionNameStatus:   1,
		SetInvalidOptionLetterStatus: 3,
	}
	out, st := run(t, "set -A\necho after\n", func(r *Runner) {
		r.Semantics, r.Diagnostics = &sem, &dg
	})
	if strings.Contains(out, "after") || st != 3 {
		t.Errorf("got %q at %d, want the letter's fatality and the letter's 3", out, st)
	}
}

// A denied `set -m` is the one refusal that arrives under either spelling, so
// it asks the axis for the spelling that asked. zsh is the only dialect that
// refuses it and answers the two alike, which is why the split has to be
// driven by hand here.
func TestADeniedMonitorAsksTheAxisForTheSpellingThatAsked(t *testing.T) {
	setup := func(r *Runner) {
		sem := CoreSemantics()
		sem.MonitorNeedsATerminal = Yes
		sem.BadSetOptionNameFatal = No
		sem.BadSetOptionLetterFatal = Yes
		sem.FatalErrorStatusIsOne = No
		r.Semantics = &sem
		r.Diagnostics = &Diagnostics{
			Location: LocationNone, MonitorDenied: "set: can't change option: %[1]s", MonitorDeniedStatus: 1,
		}
	}
	out, st := run(t, "set -o monitor\necho after\n", setup)
	if want := "sh: set: can't change option: monitor\nafter\n"; out != want || st != 0 {
		t.Errorf("the name: got %q at %d, want %q at 0", out, st, want)
	}
	out, st = run(t, "set -m\necho after\n", setup)
	if want := "sh: set: can't change option: -m\n"; out != want || st != 1 {
		t.Errorf("the letter: got %q at %d, want %q at 1", out, st, want)
	}
}
