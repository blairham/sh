// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
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
