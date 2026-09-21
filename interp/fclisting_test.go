// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"fmt"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// `fc` against a list the core reaches through the seams a dialect fills in.
//
// Nothing here names a shell: the subject is that the builtin reads
// [Runner.SetHistoryStore]'s list, counts back from the line
// [Runner.SetHistoryOwnLine] says is its own, and writes it in the shape
// [Runner.SetHistoryListingLayout] was given. What bash writes is asserted in
// dialect/bash, where a measurement belongs.

// fcList is a history list a test owns, standing in for the array a dialect
// keeps one in.
type fcList struct {
	entries []string
	// own says the list already holds the line the builtin is written on,
	// which is the state a front end reading a script leaves behind.
	own bool
}

// install fills in the three seams, the way a dialect's registration does.
func (l *fcList) install(r *Runner) {
	r.SetHistoryStore(
		func(*Runner) []string { return append([]string(nil), l.entries...) },
		func(_ *Runner, line string) { l.entries = append(l.entries, line) },
	)
	r.SetHistoryOwnLine(
		func(*Runner) bool { return l.own },
		func(*Runner) {
			if l.own && len(l.entries) > 0 {
				l.entries = l.entries[:len(l.entries)-1]
			}
		},
	)
}

func fcRun(t *testing.T, list *fcList, src string) (string, int) {
	t.Helper()
	return run(t, src, list.install)
}

// The list is read, which is the whole of #4009: the entries were there and
// the builtin never looked at them.
func TestFcListsWhatTheStoreSeamHandsOver(t *testing.T) {
	list := &fcList{entries: []string{"alpha", "beta", "gamma"}}
	out, st := fcRun(t, list, "fc -l")
	if want := "1\t alpha\n2\t beta\n3\t gamma\n"; out != want || st != 0 {
		t.Errorf("out = %q status = %d, want %q at 0", out, st, want)
	}
}

// `-n` drops the number and `-r` turns the order around, and the two are
// independent of each other.
func TestFcListingDropsTheNumberAndReversesTheOrder(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"fc -nl", "\t alpha\n\t beta\n\t gamma\n"},
		{"fc -lr", "3\t gamma\n2\t beta\n1\t alpha\n"},
		{"fc -nlr", "\t gamma\n\t beta\n\t alpha\n"},
	} {
		list := &fcList{entries: []string{"alpha", "beta", "gamma"}}
		out, st := fcRun(t, list, c.src)
		if out != c.want || st != 0 {
			t.Errorf("%s: out = %q status = %d, want %q at 0", c.src, out, st, c.want)
		}
	}
}

// An operand that looks like an option is an operand.
//
// The defect this guards is the one a suite trips over first: the option
// reader took `-2` and the line was refused where a real shell runs it. So
// the assertion is on the *range*, and a second one on there being no
// complaint at all — a builtin that listed the whole list and said nothing
// would pass a test that only looked for the refusal.
func TestAnOperandCountingBackIsNotAnOption(t *testing.T) {
	list := &fcList{entries: []string{"alpha", "beta", "gamma"}}
	out, st := fcRun(t, list, "fc -l -2")
	if want := "2\t beta\n3\t gamma\n"; out != want || st != 0 {
		t.Errorf("out = %q status = %d, want %q at 0", out, st, want)
	}
	if strings.Contains(out, "invalid option") {
		t.Error("the operand was read as an option")
	}
}

// And a dash-word that is not all digits is still options, so a real typo is
// still refused.
func TestADashWordThatIsNotANumberIsStillOptions(t *testing.T) {
	list := &fcList{entries: []string{"alpha"}}
	out, st := fcRun(t, list, "fc -l -2x")
	if !strings.Contains(out, "invalid option") || st == 0 {
		t.Errorf("out = %q status = %d, want a refusal at nonzero", out, st)
	}
}

// A range written backwards prints backwards, which is not the same question
// as `-r` and is why the two are settled in that order.
func TestARangeWrittenBackwardsPrintsBackwards(t *testing.T) {
	list := &fcList{entries: []string{"alpha", "beta", "gamma", "delta"}}
	out, _ := fcRun(t, list, "fc -l 3 1")
	if want := "3\t gamma\n2\t beta\n1\t alpha\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
}

// A word that is not a number is the newest entry beginning with it, and one
// no entry begins with is a refusal rather than an empty listing.
func TestAWordOperandFindsTheNewestEntryBeginningWithIt(t *testing.T) {
	list := &fcList{entries: []string{"beta one", "alpha", "beta two"}}
	out, st := fcRun(t, list, "fc -l beta")
	if want := "3\t beta two\n"; out != want || st != 0 {
		t.Errorf("out = %q status = %d, want %q at 0", out, st, want)
	}
	list = &fcList{entries: []string{"alpha"}}
	out, st = fcRun(t, list, "fc -l nosuch")
	if want := "fc: no command found\n"; !strings.Contains(out, want) || st != 1 {
		t.Errorf("out = %q status = %d, want %q at 1", out, st, want)
	}
}

// The layout is the dialect's and the engine is not, which is the split this
// file exists to hold: the same range, written two ways.
func TestTheListingLayoutIsTheDialectsAndTheRangeIsNot(t *testing.T) {
	list := &fcList{entries: []string{"alpha", "beta"}}
	out, _ := run(t, "fc -l -1\nfc -nl -1", func(r *Runner) {
		list.install(r)
		r.SetHistoryListingLayout("%5d  %s\n", "%s\n")
	})
	if want := "    2  beta\nbeta\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
}

// The line the builtin is written on ends the default range rather than
// joining it, and only where the dialect says the list holds it.
//
// Both halves, because either alone reads as the other: with the line in the
// list the newest entry is not an answer, and with no line in it the newest
// entry is the only answer there is.
func TestTheBuiltinsOwnLineEndsTheDefaultRange(t *testing.T) {
	kept := &fcList{entries: []string{"alpha", "beta", "fc -l"}, own: true}
	out, _ := fcRun(t, kept, "fc -l")
	if want := "1\t alpha\n2\t beta\n"; out != want {
		t.Errorf("with its own line: out = %q, want %q", out, want)
	}
	left := &fcList{entries: []string{"alpha", "beta"}}
	out, _ = fcRun(t, left, "fc -l")
	if want := "1\t alpha\n2\t beta\n"; out != want {
		t.Errorf("without it: out = %q, want %q", out, want)
	}
}

// `-s` runs an entry again, says which one on standard error, and puts it in
// the list **in place of** the call — so the list grows by nothing.
func TestRerunReplacesTheCallInTheList(t *testing.T) {
	list := &fcList{entries: []string{"echo one", "fc -s"}, own: true}
	out, st := fcRun(t, list, "fc -s")
	if want := "echo one\none\n"; out != want || st != 0 {
		t.Errorf("out = %q status = %d, want %q at 0", out, st, want)
	}
	if got := strings.Join(list.entries, "|"); got != "echo one|echo one" {
		t.Errorf("the list is %q, want the call replaced by what it ran", got)
	}
}

// `pat=rep` replaces every occurrence, and several apply in turn.
func TestRerunSubstitutesEveryOccurrence(t *testing.T) {
	list := &fcList{entries: []string{"echo aa ab"}}
	out, _ := fcRun(t, list, "fc -s a=x b=y")
	if want := "echo xx xy\nxx xy\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
}

// `-e -` is `-s` written the way POSIX spells it, and reaches the same road.
func TestTheEditorNamedDashIsTheRerun(t *testing.T) {
	list := &fcList{entries: []string{"echo one"}}
	out, _ := fcRun(t, list, "fc -e - -1")
	if want := "echo one\none\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
}

// An event that would be this very command is refused rather than run, which
// is the one thing a re-runner must never get wrong.
func TestRerunningItselfIsRefused(t *testing.T) {
	list := &fcList{entries: []string{"echo one", "fc -s -0"}, own: true}
	out, st := fcRun(t, list, "fc -s -0")
	if want := "fc: no command found\n"; !strings.Contains(out, want) || st != 1 {
		t.Errorf("out = %q status = %d, want %q at 1", out, st, want)
	}
	if got := strings.Join(list.entries, "|"); got != "echo one|fc -s -0" {
		t.Errorf("the list is %q, want it untouched", got)
	}
}

// A dialect with no list at all is unchanged: nothing is listed and nothing
// is said, which is what a shell without the feature does anyway.
func TestFcOnAShellWithNoListSaysNothing(t *testing.T) {
	out, st := run(t, "fc -l", nil)
	if out != "" || st != 0 {
		t.Errorf("out = %q status = %d, want silence at 0", out, st)
	}
}

// The two readings of an operand, each asserted over the same list.
//
// Both axes are set on every run below rather than one, because the reading
// is one question with two halves: a run that left either unanswered would be
// refused before it reached the list, and a run that left either at the other
// value would be measuring the pair rather than the half it names.
func fcReadingRun(t *testing.T, list *fcList, src string, refuse, ownEvent Answer) (string, int) {
	t.Helper()
	return run(t, src, func(r *Runner) {
		list.install(r)
		sem := *r.Semantics
		sem.FcEventOutOfRangeIsAnError = refuse
		sem.FcRelativeEventNeedsTheShellsOwnEventNumber = ownEvent
		r.Semantics = &sem
	})
}

func fcFive() *fcList {
	return &fcList{entries: []string{"alpha", "beta", "gamma", "delta", "epsilon"}}
}

// An operand the list cannot reach: brought to the nearest end, or refused.
//
// The whole listing is compared and the status with it, because the two
// answers differ in *both* and each alone reads as the other: a refusal that
// still listed would pass a status check, and a clamp that said nothing at 1
// would pass an output check.
//
// The refusal is asked of the range and not of either end, which is the third
// case here: an operand past the newest entry, paired with one inside the
// list, is clamped and nothing is said.
func TestAnEventOutOfRangeIsRefusedOrClamped(t *testing.T) {
	const whole = "1\t alpha\n2\t beta\n3\t gamma\n4\t delta\n5\t epsilon\n"
	for _, c := range []struct {
		refuse Answer
		src    string
		want   string
		status int
	}{
		{No, "fc -l 99", whole, 0},
		{Yes, "fc -l 99", "sh: fc: no such event: 99\n", 1},
		// Two ends that resolved to one number name it; two that differ
		// cannot, and say only that the range is empty.
		{Yes, "fc -l 6 6", "sh: fc: no such event: 6\n", 1},
		{Yes, "fc -l 6 7", "sh: fc: no events in that range\n", 1},
		// And a range that still meets the list is not refused at either
		// end: the 6 is clamped to the newest entry, silently.
		{Yes, "fc -l 6 3", "5\t epsilon\n4\t delta\n3\t gamma\n", 0},
		// And where an operand out of range is *clamped*, the 6 was never a
		// number the range kept: it fell back to the start of the default
		// window, which is below the list, so the range runs 1 to 3.
		{No, "fc -l 6 3", "1\t alpha\n2\t beta\n3\t gamma\n", 0},
	} {
		out, st := fcReadingRun(t, fcFive(), c.src, c.refuse, No)
		if out != c.want || st != c.status {
			t.Errorf("%s under %v: out = %q status = %d, want %q at %d",
				c.src, c.refuse, out, st, c.want, c.status)
		}
	}
}

// A relative operand: counted back from the end of the list, or counted back
// from a current event the shell reading a script has none of.
//
// The three magnitudes together are the assertion, because the second answer
// is exactly that they cannot be told apart — a run that only checked `-1`
// could not see the difference between "the whole list" and "one entry, and
// the list happens to be one entry long".
func TestARelativeOperandCountsFromTheEndOrFromNothing(t *testing.T) {
	const whole = "1\t alpha\n2\t beta\n3\t gamma\n4\t delta\n5\t epsilon\n"
	for _, c := range []struct {
		ownEvent Answer
		src      string
		want     string
	}{
		{No, "fc -l -1", "5\t epsilon\n"},
		{No, "fc -l -2", "4\t delta\n5\t epsilon\n"},
		{No, "fc -l -20", whole},
		{Yes, "fc -l -1", whole},
		{Yes, "fc -l -2", whole},
		{Yes, "fc -l -20", whole},
		// `0` is where the count back from nothing already ends, so it is
		// that same operand there; it is the entry before this line where
		// there is a line to count back from.
		{No, "fc -l 0", "5\t epsilon\n"},
		{Yes, "fc -l 0", whole},
	} {
		out, st := fcReadingRun(t, fcFive(), c.src, No, c.ownEvent)
		if out != c.want || st != 0 {
			t.Errorf("%s under %v: out = %q status = %d, want %q at 0",
				c.src, c.ownEvent, out, st, c.want)
		}
	}
	// `-0` parts the two readings the other way: a count of none where there
	// is something to count from, and a word no entry begins with where
	// there is not.
	out, st := fcReadingRun(t, fcFive(), "fc -l -0", No, No)
	if want := "5\t epsilon\n"; out != want || st != 0 {
		t.Errorf("-0 counting back: out = %q status = %d, want %q at 0", out, st, want)
	}
	out, st = fcReadingRun(t, fcFive(), "fc -l -0", No, Yes)
	if want := "sh: fc: no command found\n"; out != want || st != 1 {
		t.Errorf("-0 as a word: out = %q status = %d, want %q at 1", out, st, want)
	}
}

// The same axis decides where a default range starts, which is why it is one
// axis and not two: a shell with a current event takes the sixteen events
// below it, and a shell with none takes the newest seventeen entries.
//
// Eighteen entries, because that is the shortest list the two answers differ
// on — on seventeen or fewer both write all of them, which is what made the
// difference invisible until a list was long enough to have a window.
func TestTheDefaultRangeStartsWhereTheCountingDoes(t *testing.T) {
	entries := make([]string, 18)
	for i := range entries {
		entries[i] = fmt.Sprintf("line %d", i+1)
	}
	for _, c := range []struct {
		ownEvent Answer
		first    int
	}{{No, 3}, {Yes, 2}} {
		out, st := fcReadingRun(t, &fcList{entries: entries}, "fc -l", No, c.ownEvent)
		var want strings.Builder
		for n := c.first; n <= 18; n++ {
			fmt.Fprintf(&want, "%d\t line %d\n", n, n)
		}
		if out != want.String() || st != 0 {
			t.Errorf("under %v: out = %q status = %d, want %q at 0", c.ownEvent, out, st, want.String())
		}
	}
}

// An empty list is not a case of its own where an event out of range is
// refused: every range misses a list with nothing in it, so the refusal is
// worded from the operands and names what was asked for.
//
// The three rows are three different answers to the same empty list, which is
// the whole of it — a shell that reported one fixed event would pass on the
// first row alone.
func TestAnEmptyListIsRefusedFromTheOperandsThatMissedIt(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"fc -l", "sh: fc: no such event: 1\n"},
		{"fc -l -1", "sh: fc: no such event: 0\n"},
		{"fc -l 99", "sh: fc: no such event: 99\n"},
		{"fc -l 2 5", "sh: fc: no events in that range\n"},
		{"fc -l nosuch", "sh: fc: no command found\n"},
	} {
		out, st := run(t, c.src, func(r *Runner) {
			(&fcList{}).install(r)
			sem := *r.Semantics
			sem.FcEmptyHistoryIsAnError = Yes
			sem.FcEventOutOfRangeIsAnError = Yes
			sem.FcRelativeEventNeedsTheShellsOwnEventNumber = Yes
			r.Semantics = &sem
		})
		if out != c.want || st != 1 {
			t.Errorf("%s: out = %q status = %d, want %q at 1", c.src, out, st, c.want)
		}
	}
}
