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
// still refused — in the reading where the options end only at a word that
// is a dash and nothing but digits. The other reading ends them at a dash and
// a digit however the word goes on, so the very same word is an operand
// there: [Semantics.FcOptionsEndAtADashAndADigit], and the two are asserted
// together because each alone reads as a shell that could not tell an option
// from an event.
func TestADashWordThatIsNotANumberIsStillOptions(t *testing.T) {
	out, st := fcCutRun(t, &fcList{entries: []string{"alpha"}}, "fc -l -2x", No)
	if !strings.Contains(out, "invalid option") || st == 0 {
		t.Errorf("out = %q status = %d, want a refusal at nonzero", out, st)
	}
	out, st = fcCutRun(t, fcFive(), "fc -l -2x", Yes)
	if want := "4\t delta\n5\t epsilon\n"; out != want || st != 0 {
		t.Errorf("out = %q status = %d, want %q at 0", out, st, want)
	}
}

// fcCutRun answers where this shell's `fc` stops reading options.
func fcCutRun(t *testing.T, list *fcList, src string, prefix Answer) (string, int) {
	t.Helper()
	return run(t, src, func(r *Runner) {
		list.install(r)
		sem := *r.Semantics
		sem.FcOptionsEndAtADashAndADigit = prefix
		r.Semantics = &sem
	})
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
	return fcThresholdRun(t, list, src, refuse, ownEvent, No)
}

// fcThresholdRun is fcReadingRun with the third reading written too: whether
// the newest entry is the line this shell is standing on, which is the one
// the roads that *run* an entry stop at.
func fcThresholdRun(t *testing.T, list *fcList, src string, refuse, ownEvent, newest Answer) (string, int) {
	t.Helper()
	return run(t, src, func(r *Runner) {
		list.install(r)
		sem := *r.Semantics
		sem.FcEventOutOfRangeIsAnError = refuse
		sem.FcRelativeEventNeedsTheShellsOwnEventNumber = ownEvent
		sem.FcNewestEntryIsTheCurrentLine = newest
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

// The threshold the roads that *run* an entry stop at: the newest entry is
// reachable, or it is the line this shell is standing on and running it
// would be running this very command again.
//
// Every row below moves one axis and nothing else, and the `-s` road is
// where the two answers are visible without an editor. The default is in the
// table beside the written operands on purpose: a threshold that only
// refused what somebody wrote down would leave `fc -s` on a one-entry list
// re-running the line it is on, which is the case the refusal exists for.
func TestTheNewestEntryIsReachableOrIsTheCurrentLine(t *testing.T) {
	const refusal = "sh: fc: the current history line would run itself again\n"
	five := func() *fcList {
		return &fcList{entries: []string{
			"echo one", "echo two", "echo three", "echo four", "echo five",
		}}
	}
	for _, c := range []struct {
		newest Answer
		src    string
		want   string
		status int
	}{
		{No, "fc -s 5", "echo five\nfive\n", 0},
		{Yes, "fc -s 5", refusal, 1},
		// The entry below it is reachable under both, so this is a
		// threshold and not a road that was switched off.
		{No, "fc -s 4", "echo four\nfour\n", 0},
		{Yes, "fc -s 4", "echo four\nfour\n", 0},
		// And the default lands on it, so an operand nobody wrote is
		// refused exactly as one somebody did.
		{No, "fc -s", "echo five\nfive\n", 0},
		{Yes, "fc -s", refusal, 1},
	} {
		out, st := fcThresholdRun(t, five(), c.src, No, No, c.newest)
		if out != c.want || st != c.status {
			t.Errorf("%s under %v: out = %q status = %d, want %q at %d",
				c.src, c.newest, out, st, c.want, c.status)
		}
	}
	// A list of one is where the default and the threshold are the same
	// entry, which is the row that says the refusal is not about the
	// operand: there is nothing below the line to fall back to.
	out, st := fcThresholdRun(t, &fcList{entries: []string{"echo only"}}, "fc -s", Yes, Yes, Yes)
	if out != refusal || st != 1 {
		t.Errorf("one entry: out = %q status = %d, want %q at 1", out, st, refusal)
	}
	// And it is answered ahead of the range refusal, which the same operand
	// tells apart: an event past the end of the list is this sentence in the
	// shell that has the threshold and is named as an event in the one that
	// only refuses the range.
	out, st = fcThresholdRun(t, five(), "fc -s 99", Yes, Yes, Yes)
	if out != refusal || st != 1 {
		t.Errorf("past the end: out = %q status = %d, want %q at 1", out, st, refusal)
	}
	out, st = fcThresholdRun(t, five(), "fc -s 99", Yes, Yes, No)
	if want := "sh: fc: no such event: 99\n"; out != want || st != 1 {
		t.Errorf("past the end without the threshold: out = %q status = %d, want %q at 1", out, st, want)
	}
}

// The same threshold stops a word operand's search, and that reaches `-l` —
// so the listing road, which can still *list* the newest entry, cannot
// *find* it by a prefix.
//
// The pair is the assertion. A run that only asked the shell with the
// threshold would read as a search that works, since `ay` is a perfectly
// good answer to `a` until you know `az` was there too.
func TestAWordOperandsSearchStopsAtTheCurrentLine(t *testing.T) {
	entries := func() *fcList {
		return &fcList{entries: []string{"ax", "bx", "ay", "by", "az"}}
	}
	out, st := fcThresholdRun(t, entries(), "fc -l a", No, No, No)
	if want := "5\t az\n"; out != want || st != 0 {
		t.Errorf("out = %q status = %d, want %q at 0", out, st, want)
	}
	out, st = fcThresholdRun(t, entries(), "fc -l a", No, No, Yes)
	if want := "3\t ay\n4\t by\n5\t az\n"; out != want || st != 0 {
		t.Errorf("out = %q status = %d, want %q at 0", out, st, want)
	}
}

// On the editor road the two ends answer the threshold differently, which is
// measured and is the one place a single rule would have been wrong: `first`
// at or past the current line is the refusal, and `last` past it is brought
// under it with nothing said.
func TestTheEditorRoadRefusesFirstAndClampsLastAtTheCurrentLine(t *testing.T) {
	five := func() *fcList {
		return &fcList{entries: []string{": one", ": two", ": three", ": four", ": five"}}
	}
	// `cat` prints the file it is handed, so what the editor saw is in the
	// output, and the shell then echoes each line back — twice for a line
	// that made it into the range and never for one that did not.
	for _, c := range []struct {
		newest Answer
		five   int
	}{{No, 2}, {Yes, 0}} {
		out, st := fcThresholdRun(t, five(), "fc -e cat 1 5", No, No, c.newest)
		if got := strings.Count(out, ": five"); got != c.five || st != 0 {
			t.Errorf("under %v the range held the newest entry %d times, want %d (out = %q, status %d)",
				c.newest, got, c.five, out, st)
		}
		if !strings.Contains(out, ": four") {
			t.Errorf("under %v the range lost the entry below the line: %q", c.newest, out)
		}
	}
	// And the same number written as `first` is the refusal rather than a
	// range brought under the line. Asserted in the reading that *keeps* an
	// out-of-range absolute, because the other one clamps the 5 to the
	// oldest entry before the threshold is ever consulted — so the pair
	// below is over the threshold alone and the run is otherwise the same.
	out, st := fcThresholdRun(t, five(), "fc -e cat 5", Yes, Yes, No)
	if want := ": five\n: five\n"; out != want || st != 0 {
		t.Errorf("out = %q status = %d, want %q at 0", out, st, want)
	}
	out, st = fcThresholdRun(t, five(), "fc -e cat 5", Yes, Yes, Yes)
	if want := "sh: fc: the current history line would run itself again\n"; out != want || st != 1 {
		t.Errorf("out = %q status = %d, want %q at 1", out, st, want)
	}
}

// An operand's number is the digits at the *front* of the word in both
// readings, and what may stand in front of those digits is the one thing
// they disagree about.
//
// The first two rows are the shared half and are asserted under both
// answers, because a fix that only reached the shell with the looser reading
// would leave `fc -l 2x` refused in the other and look right from either
// side alone.
func TestAnOperandsNumberIsReadWithOrWithoutWhatStandsInFrontOfIt(t *testing.T) {
	const noCommand = "sh: fc: no command found\n"
	for _, c := range []struct {
		loose  Answer
		src    string
		want   string
		status int
	}{
		// The prefix, which neither shell disputes and no axis governs.
		{No, "fc -l 2x", "2\t beta\n3\t gamma\n4\t delta\n5\t epsilon\n", 0},
		{Yes, "fc -l 2x", "2\t beta\n3\t gamma\n4\t delta\n5\t epsilon\n", 0},
		{No, "fc -l 0x2", "5\t epsilon\n", 0},
		{Yes, "fc -l 0x2", "5\t epsilon\n", 0},
		// Whitespace in front of the digits: part of the number, or the
		// start of a word no entry begins with.
		{No, "fc -l ' 2'", noCommand, 1},
		{Yes, "fc -l ' 2'", "2\t beta\n3\t gamma\n4\t delta\n5\t epsilon\n", 0},
		{No, "fc -l ' -1'", noCommand, 1},
		{Yes, "fc -l ' -1'", "5\t epsilon\n", 0},
		// And a leading `+`, which is the same disagreement without any
		// whitespace in it.
		{No, "fc -l +3", noCommand, 1},
		{Yes, "fc -l +3", "3\t gamma\n4\t delta\n5\t epsilon\n", 0},
		// A word with one of them and no digits behind it is a search in
		// both, so the looser reading is not "anything goes".
		{No, "fc -l ' zz'", noCommand, 1},
		{Yes, "fc -l ' zz'", noCommand, 1},
	} {
		out, st := fcLooseRun(t, fcFive(), c.src, c.loose)
		if out != c.want || st != c.status {
			t.Errorf("%s under %v: out = %q status = %d, want %q at %d",
				c.src, c.loose, out, st, c.want, c.status)
		}
	}
}

// fcLooseRun answers whether this shell's `fc` reads an operand's digits past
// whitespace and a `+`.
func fcLooseRun(t *testing.T, list *fcList, src string, loose Answer) (string, int) {
	t.Helper()
	return run(t, src, func(r *Runner) {
		list.install(r)
		sem := *r.Semantics
		sem.FcNumericOperandSkipsBlanksAndASign = loose
		r.Semantics = &sem
	})
}

// An operand written as the empty string is a **search**, not an absent
// operand — and every entry begins with the empty string, so it names the
// newest entry the search is allowed to reach.
//
// One rule in both readings, which is why no axis is moved below: what
// differs is only where the search stops, and that is the threshold above.
// The absent operand is asserted beside each empty one, because the bug this
// holds off is exactly the two being read alike — a run that only checked
// `fc -l ”` against a listing could not tell "the empty word searched" from
// "the empty word was ignored" on a short list.
func TestAnEmptyOperandIsASearchAndNotAnAbsentOne(t *testing.T) {
	const whole = "1\t alpha\n2\t beta\n3\t gamma\n4\t delta\n5\t epsilon\n"
	for _, c := range []struct{ src, want string }{
		// `first` written empty is the newest entry; absent it is the
		// sixteen-back window, which on five entries is the whole list.
		{"fc -l ''", "5\t epsilon\n"},
		{"fc -l", whole},
		// `last` written empty ends the range at that same entry; absent
		// it ends at cur-1, which here is the same place — so the pair
		// that separates them is the one above and not this one.
		{"fc -l 2 ''", "2\t beta\n3\t gamma\n4\t delta\n5\t epsilon\n"},
		{"fc -l '' ''", "5\t epsilon\n"},
	} {
		out, st := fcThresholdRun(t, fcFive(), c.src, No, No, No)
		if out != c.want || st != 0 {
			t.Errorf("%s: out = %q status = %d, want %q at 0", c.src, out, st, c.want)
		}
	}
	// And where the search stops below the current line, the empty word
	// stops with it — the same one rule, read through the other threshold.
	out, st := fcThresholdRun(t, fcFive(), "fc -l ''", No, No, Yes)
	if want := "4\t delta\n5\t epsilon\n"; out != want || st != 0 {
		t.Errorf("out = %q status = %d, want %q at 0", out, st, want)
	}
}

// The `-s` road reads its operand through a second splitter, which had the
// same bug for the same reason: an empty word and no word at all were one
// state there too.
func TestAnEmptyOperandUnderRerunIsASearch(t *testing.T) {
	five := func() *fcList {
		return &fcList{entries: []string{
			"echo one", "echo two", "echo three", "echo four", "echo five",
		}}
	}
	// Written empty: the newest entry. Absent: the same here, which is why
	// the row below it — a substitution in front of the empty word — is the
	// one that separates the two states.
	out, st := fcThresholdRun(t, five(), "fc -s ''", No, No, No)
	if want := "echo five\nfive\n"; out != want || st != 0 {
		t.Errorf("out = %q status = %d, want %q at 0", out, st, want)
	}
	// A `pat=rep` operand is not the command word, so the empty word behind
	// it is still the operand and the substitution still applies to what it
	// found.
	out, st = fcThresholdRun(t, five(), "fc -s five=FIVE ''", No, No, No)
	if want := "echo FIVE\nFIVE\n"; out != want || st != 0 {
		t.Errorf("out = %q status = %d, want %q at 0", out, st, want)
	}
	// And the same word where the search stops below the current line.
	out, st = fcThresholdRun(t, five(), "fc -s ''", No, No, Yes)
	if want := "echo four\nfour\n"; out != want || st != 0 {
		t.Errorf("out = %q status = %d, want %q at 0", out, st, want)
	}
}

// A range whose entries would run newest first: run in that order, or
// refused.
//
// The judgement is on the order they would **run** in, which `-r` is what
// separates — so the four rows below are two pairs rather than four cases,
// and a rule written on the two operands passes the first pair and fails the
// second.
func TestARangeThatWouldRunBackwardsIsRunOrRefused(t *testing.T) {
	const refusal = "sh: fc: a range of history events cannot be run newest first\n"
	five := func() *fcList {
		return &fcList{entries: []string{": one", ": two", ": three", ": four", ": five"}}
	}
	for _, c := range []struct {
		refuse Answer
		src    string
		file   string
		status int
	}{
		// Written backwards, with nothing turning it around.
		{No, "fc -e cat 3 1", ": three\n: two\n: one\n", 0},
		{Yes, "fc -e cat 3 1", "", 1},
		// Written forwards and turned around by `-r`, which is the same
		// run order reached the other way.
		{No, "fc -r -e cat 1 3", ": three\n: two\n: one\n", 0},
		{Yes, "fc -r -e cat 1 3", "", 1},
		// Written backwards and turned around again, which runs forwards
		// and is refused by neither.
		{No, "fc -r -e cat 3 1", ": one\n: two\n: three\n", 0},
		{Yes, "fc -r -e cat 3 1", ": one\n: two\n: three\n", 0},
		// A single event is not a backwards range under either.
		{No, "fc -e cat 2 2", ": two\n", 0},
		{Yes, "fc -e cat 2 2", ": two\n", 0},
	} {
		out, st := fcBackwardsRun(t, five(), c.src, c.refuse)
		if c.file == "" {
			if out != refusal || st != c.status {
				t.Errorf("%s under %v: out = %q status = %d, want %q at %d",
					c.src, c.refuse, out, st, refusal, c.status)
			}
			continue
		}
		// `cat` prints the file, and the shell then echoes each line back,
		// so what the editor saw is the front of the output.
		if !strings.HasPrefix(out, c.file) || st != c.status {
			t.Errorf("%s under %v: out = %q status = %d, want it to open with %q at %d",
				c.src, c.refuse, out, st, c.file, c.status)
		}
	}
}

// fcBackwardsRun answers whether this shell runs a range newest first.
func fcBackwardsRun(t *testing.T, list *fcList, src string, refuse Answer) (string, int) {
	t.Helper()
	return run(t, src, func(r *Runner) {
		list.install(r)
		sem := *r.Semantics
		sem.FcBackwardsRangeIsAnError = refuse
		r.Semantics = &sem
	})
}

// The three refusals the editor road can give, in the order a matrix put
// them in: both ends past the current line is the recursion sentence, one
// end past it is the backwards one, and both ends below the list is the
// range one.
//
// They are asserted together because each was measured by the case that
// tells it from the one beside it — `5 5` against `5 4` is the whole of why
// the recursion check reads both ends rather than `first`.
func TestTheEditorRoadsThreeRefusalsAreOrdered(t *testing.T) {
	five := func() *fcList {
		return &fcList{entries: []string{": one", ": two", ": three", ": four", ": five"}}
	}
	for _, c := range []struct{ src, want string }{
		{"fc -e cat 5 5", "sh: fc: the current history line would run itself again\n"},
		{"fc -e cat 6 7", "sh: fc: the current history line would run itself again\n"},
		{"fc -e cat 7 6", "sh: fc: the current history line would run itself again\n"},
		{"fc -e cat 5 4", "sh: fc: a range of history events cannot be run newest first\n"},
		{"fc -e cat 99 1", "sh: fc: a range of history events cannot be run newest first\n"},
		{"fc -e cat 0 0", "sh: fc: no such event: 0\n"},
	} {
		out, st := run(t, c.src, func(r *Runner) {
			list := five()
			list.install(r)
			sem := *r.Semantics
			sem.FcEventOutOfRangeIsAnError = Yes
			sem.FcRelativeEventNeedsTheShellsOwnEventNumber = Yes
			sem.FcNewestEntryIsTheCurrentLine = Yes
			sem.FcBackwardsRangeIsAnError = Yes
			r.Semantics = &sem
		})
		if out != c.want || st != 1 {
			t.Errorf("%s: out = %q status = %d, want %q at 1", c.src, out, st, c.want)
		}
	}
	// And the range that reaches the newest entry only through `-r`, which
	// is the pair that says the bound is the operand's place and not the
	// range's: `4 5` edits one entry and `-r 5 4` edits two.
	for _, c := range []struct{ src, file string }{
		{"fc -e cat 4 5", ": four\n"},
		{"fc -r -e cat 5 4", ": four\n: five\n"},
	} {
		out, st := run(t, c.src, func(r *Runner) {
			list := five()
			list.install(r)
			sem := *r.Semantics
			sem.FcEventOutOfRangeIsAnError = Yes
			sem.FcRelativeEventNeedsTheShellsOwnEventNumber = Yes
			sem.FcNewestEntryIsTheCurrentLine = Yes
			sem.FcBackwardsRangeIsAnError = Yes
			r.Semantics = &sem
		})
		if !strings.HasPrefix(out, c.file) || st != 0 {
			t.Errorf("%s: out = %q status = %d, want it to open with %q at 0", c.src, out, st, c.file)
		}
	}
}
