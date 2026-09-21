// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// How this shell reads `fc -l`'s operands, which is not how bash reads them.
//
// Every expected string below is a transcript. Each script was run through
// zsh 5.9.2 on 2026-09-21 as a **script file** — `zsh -f` under `env -i` with
// a scratch `HOME` — with the list planted by `print -s` lines, because this
// shell records nothing a script runs and a `print -s` entry is the only kind
// a script can put there. The `fc` line itself is therefore never in the
// list, which is the whole reason the two readings differ: there is no
// current event to count an operand back from. #4018.
//
// The location a refusal carries is `<file>:fc:<line>:` in the real shell and
// the same shape here, so the transcripts below are the text after it.

// fcListRun runs a script whose list has n entries called `e1`..`en`, and
// returns everything it wrote.
func fcListRun(t *testing.T, n int, line string) (string, int) {
	t.Helper()
	var src strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&src, "print -s e%d\n", i)
	}
	src.WriteString(line + "\n")
	out, st, err := preset.Combined(t, dialecttest.Base{}, src.String())
	if err != nil {
		t.Fatalf("run %q: %v", line, err)
	}
	return out, st
}

// fcLines is the listing of the entries from a to b inclusive, in the shape
// this shell writes: five columns for the number, two spaces, the command.
func fcLines(a, b int) string {
	var want strings.Builder
	step := 1
	if a > b {
		step = -1
	}
	for n := a; ; n += step {
		fmt.Fprintf(&want, "%5d  e%d\n", n, n)
		if n == b {
			break
		}
	}
	return want.String()
}

// A relative operand names nothing in the list, whatever its magnitude.
//
// The three magnitudes on two list lengths, because the claim is that they
// cannot be told apart: `-1` on a five-entry list writes the same five lines
// that `-20` does, and on thirty entries all three write thirty. bash's `-1`
// is the newest entry and its `-20` the newest twenty.
func TestARelativeOperandNamesNothingInAScriptsList(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		n    int
		line string
	}{
		{5, "fc -l -1"},
		{5, "fc -l -2"},
		{5, "fc -l -20"},
		{5, "fc -l 0"},
		{30, "fc -l -1"},
		{30, "fc -l -2"},
		{30, "fc -l -20"},
	} {
		out, st := fcListRun(t, c.n, c.line)
		if want := fcLines(1, c.n); out != want || st != 0 {
			t.Errorf("%d entries, %q: out = %q status = %d, want %q at 0",
				c.n, c.line, out, st, want)
		}
	}
}

// `-0` is not one of them: it is a word, and no entry begins with it.
func TestTheOperandMinusZeroIsAWordAndNotACount(t *testing.T) {
	t.Parallel()
	out, st := fcListRun(t, 5, "fc -l -0")
	if want := "zsh:fc:6: event not found: -0\n"; out != want || st != 1 {
		t.Errorf("out = %q status = %d, want %q at 1", out, st, want)
	}
}

// An event past the list is refused rather than brought to the nearest end,
// and the refusal names it.
//
// `fc -l 6` and `fc -l 6 6` are the same refusal by two roads — an absent
// `last` is the later of the newest entry and `first`, so a `first` past the
// newest is both ends of the range.
func TestAnEventPastTheListIsRefused(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		line, want string
		status     int
	}{
		{"fc -l 99", "zsh:fc:6: no such event: 99\n", 1},
		{"fc -l 6", "zsh:fc:6: no such event: 6\n", 1},
		{"fc -l 6 6", "zsh:fc:6: no such event: 6\n", 1},
		{"fc -lr 99", "zsh:fc:6: no such event: 99\n", 1},
		{"fc -l 007", "zsh:fc:6: no such event: 7\n", 1},
		// Two ends that resolved to different numbers name neither.
		{"fc -l 6 7", "zsh:fc:6: no events in that range\n", 1},
		{"fc -l 7 6", "zsh:fc:6: no events in that range\n", 1},
		// Both ends below the list is the same refusal on the other side,
		// and the number it names is where a count back from nothing stops.
		{"fc -l 0 0", "zsh:fc:6: no such event: 0\n", 1},
		{"fc -l -1 -1", "zsh:fc:6: no such event: 0\n", 1},
		{"fc -l -2 -2", "zsh:fc:6: no such event: 0\n", 1},
		{"fc -l -1 -2", "zsh:fc:6: no such event: 0\n", 1},
		// A word no entry begins with is a third wording again.
		{"fc -l zzz", "zsh:fc:6: event not found: zzz\n", 1},
		{"fc -l 2 zzz", "zsh:fc:6: event not found: zzz\n", 1},
	} {
		out, st := fcListRun(t, 5, c.line)
		if out != c.want || st != c.status {
			t.Errorf("%q: out = %q status = %d, want %q at %d", c.line, out, st, c.want, c.status)
		}
	}
}

// A range that still meets the list is not refused at either end.
//
// This is what makes the refusal a statement about the range rather than
// about an operand, and it is the half a status-only check would miss: every
// row here is silence at 0 with an operand the list does not hold in it.
func TestARangeThatStillMeetsTheListIsClamped(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ line, want string }{
		{"fc -l 6 3", fcLines(5, 3)},
		{"fc -l 3 6", fcLines(3, 5)},
		{"fc -l 99 0", fcLines(5, 1)},
		{"fc -l 0 99", fcLines(1, 5)},
		{"fc -l 99 -1", fcLines(5, 1)},
		{"fc -l -1 99", fcLines(1, 5)},
		{"fc -l 3 -1", fcLines(3, 1)},
		{"fc -l 1 99", fcLines(1, 5)},
	} {
		out, st := fcListRun(t, 5, c.line)
		if out != c.want || st != 0 {
			t.Errorf("%q: out = %q status = %d, want %q at 0", c.line, out, st, c.want)
		}
	}
}

// The default range is the newest seventeen entries, counted on the list,
// because there is no current event to take sixteen off.
//
// Seventeen and eighteen are the pair that says so: below eighteen every
// answer writes the whole list, and bash's window on the same eighteen starts
// one entry later than this one's.
func TestTheDefaultRangeIsTheNewestSeventeenEntries(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ n, first int }{
		{5, 1}, {17, 1}, {18, 2}, {30, 14},
	} {
		out, st := fcListRun(t, c.n, "fc -l")
		if want := fcLines(c.first, c.n); out != want || st != 0 {
			t.Errorf("%d entries: out = %q status = %d, want %q at 0", c.n, out, st, want)
		}
	}
}

// An empty list is refused from the operands that missed it, and not with one
// fixed event: the same empty list answers four ways.
func TestAnEmptyListIsRefusedFromTheOperandThatMissedIt(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ line, want string }{
		{"fc -l", "zsh:fc:1: no such event: 1\n"},
		{"fc -nl", "zsh:fc:1: no such event: 1\n"},
		{"fc -l 1", "zsh:fc:1: no such event: 1\n"},
		{"fc -l -1", "zsh:fc:1: no such event: 0\n"},
		{"fc -l 99", "zsh:fc:1: no such event: 99\n"},
		{"fc -l 2 5", "zsh:fc:1: no events in that range\n"},
		{"fc -l zzz", "zsh:fc:1: event not found: zzz\n"},
	} {
		out, st := fcListRun(t, 0, c.line)
		if out != c.want || st != 1 {
			t.Errorf("%q: out = %q status = %d, want %q at 1", c.line, out, st, c.want)
		}
	}
}

// `fc -s` with no operand runs the **oldest** entry, which is the same
// reading arriving on the re-run road: the default counts back from a current
// event this shell has none of, and a default — unlike a written operand — is
// brought into the list rather than refused.
//
// The pair is the assertion. `fc -s 0` is that number written down and is
// refused, so a run that only checked the default could not tell "brought
// into the list" from "the list starts there anyway".
func TestARerunWithNoOperandTakesTheOldestEntry(t *testing.T) {
	t.Parallel()
	const seed = "print -s 'echo one'\nprint -s 'echo two'\nprint -s 'echo three'\n"
	for _, c := range []struct {
		line, want string
		status     int
	}{
		{"fc -s", "echo one\none\n", 0},
		{"fc -s 2", "echo two\ntwo\n", 0},
		{"fc -s 0", "zsh:fc:4: no such event: 0\n", 1},
		{"fc -s -1", "zsh:fc:4: no such event: 0\n", 1},
	} {
		out, st, err := preset.Combined(t, dialecttest.Base{}, seed+c.line+"\n")
		if err != nil {
			t.Fatalf("run %q: %v", c.line, err)
		}
		if out != c.want || st != c.status {
			t.Errorf("%q: out = %q status = %d, want %q at %d", c.line, out, st, c.want, c.status)
		}
	}
}
