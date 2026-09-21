// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// Assigning HISTSIZE trims a list that is already longer than it, and the
// entries it keeps are renumbered from the count it dropped.
//
// Measured 2026-09-21 against bash 5.3.20 at `/opt/homebrew/bin/bash` — the
// panel's bash, not `/bin/bash`, which is 3.2 — with `env -i` and a scratch
// HOME (#4031). Every want below is the byte-for-byte output of the same
// script under the real binary, and every row was run side by side with it.
//
// The cap on the way in already agreed, which is why this was invisible to
// anything that sets HISTSIZE before building a list; it was the assignment
// that reached a longer list and did nothing.

// historyStifleIgnores keeps the reader's own lines out of the list, so the
// only entries are the ones `history -s` stored.
//
// Not tidiness: the line carrying the assignment is itself an entry, and its
// insert moves the numbers on again before the `history` on the next line can
// look — which is exactly what hid the rule. With the reader quiet, what the
// assignment did to the list is what the listing shows.
const historyStifleIgnores = "HISTIGNORE='history*:HISTSIZE*:set*:unset*:HISTIGNORE*:" +
	"echo*:export*:declare*:typeset*:printf*:local*:fn*:((*'\nset -o history\n"

// historyStifleRun runs a script under those ignores and answers the whole
// listing, the diagnostics and the status.
//
// The whole listing, numbers included, because the numbering is half of what
// is being asserted here: a containment check passes for a list that kept the
// right entries under the wrong numbers, and the two routes to a two-entry
// list differ by nothing else.
func historyStifleRun(t *testing.T, src string) (string, string, int) {
	t.Helper()
	return historyRun(t, historyStifleIgnores+src)
}

// historyStifleAdds is `history -s` over the first n of `a`, `b`, `c`, …
func historyStifleAdds(n int) string {
	var b strings.Builder
	for i := range n {
		b.WriteString("history -s " + string(rune('a'+i)) + "\n")
	}
	return b.String()
}

// The trim, and the numbering it leaves behind. The oldest entry it keeps is
// numbered by **how many it dropped** — four of nine leaves the oldest at 4,
// seven of nine leaves it at 7 — whatever the numbers were beforehand, which
// the fourth and fifth rows show by starting from a list already renumbered
// once.
func TestAssigningHistsizeTrimsAndRenumbersFromWhatItDropped(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{
			"the issue's own case",
			historyStifleAdds(3) + "HISTSIZE=2\nhistory\n",
			"    1  b\n    2  c\n",
		},
		{
			"four of nine dropped",
			historyStifleAdds(9) + "HISTSIZE=5\nhistory\n",
			"    4  e\n    5  f\n    6  g\n    7  h\n    8  i\n",
		},
		{
			"one of nine dropped",
			historyStifleAdds(9) + "HISTSIZE=8\nhistory\n",
			"    1  b\n    2  c\n    3  d\n    4  e\n    5  f\n    6  g\n    7  h\n    8  i\n",
		},
		{
			"a second trim numbers from its own drop",
			historyStifleAdds(9) + "HISTSIZE=5\nHISTSIZE=2\nhistory\n",
			"    3  h\n    4  i\n",
		},
		{
			"and from numbers the insert path moved",
			"HISTSIZE=3\n" + historyStifleAdds(5) + "HISTSIZE=2\nhistory\n",
			"    1  d\n    2  e\n",
		},
		{
			"down to one",
			historyStifleAdds(5) + "HISTSIZE=2\nHISTSIZE=1\nhistory\n",
			"    1  e\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, errs, code := historyStifleRun(t, c.src)
			if out != c.want || errs != "" || code != 0 {
				t.Errorf("out %q err %q status %d, want %q, no diagnostic and 0", out, errs, code, c.want)
			}
		})
	}
}

// The same rule at its limit: zero empties the list, and the count still
// decides where the numbering picks up. Pinned on its own because an
// off-by-one in the trim reads as correct everywhere except here.
func TestAssigningHistsizeZeroEmptiesTheList(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{"nothing is left", historyStifleAdds(3) + "HISTSIZE=0\nhistory\n", ""},
		{
			"and the next entry is numbered by the drop",
			historyStifleAdds(2) + "HISTSIZE=0\nHISTSIZE=9\nhistory -s c\nhistory\n",
			"    2  c\n",
		},
		{
			"five dropped, so the next is the fifth",
			historyStifleAdds(5) + "HISTSIZE=0\nHISTSIZE=9\nhistory -s f\nhistory\n",
			"    5  f\n",
		},
		{
			"three dropped off a list the insert path had already shortened",
			"HISTSIZE=3\n" + historyStifleAdds(5) + "HISTSIZE=0\nHISTSIZE=9\nhistory -s f\nhistory\n",
			"    3  f\n",
		},
		{
			"an emptied list is not a cleared one: -c starts over at 1",
			historyStifleAdds(3) + "history -c\nHISTSIZE=9\nhistory -s d\nhistory\n",
			"    1  d\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, errs, code := historyStifleRun(t, c.src)
			if out != c.want || errs != "" || code != 0 {
				t.Errorf("out %q err %q status %d, want %q, no diagnostic and 0", out, errs, code, c.want)
			}
		})
	}
}

// A list at or under the new size keeps every entry **and** every number, and
// so does one whose value is not a count. The two halves are one test because
// the failure they guard against is the same: a trim that fires where bash
// leaves the list alone is invisible until the numbers are read.
func TestAnAssignmentThatIsNotATrimLeavesTheNumbersAlone(t *testing.T) {
	t.Parallel()
	const untouched = "    1  a\n    2  b\n    3  c\n"
	for _, c := range []struct{ name, value, want string }{
		{"exactly at the list's length", "3", untouched},
		{"above it", "5", untouched},
		{"a negative keeps everything", "-1", untouched},
		{"and so does an empty value", "", untouched},
		{"letters are not a count", "abc", untouched},
		{"nor are digits with text after them", "2x", untouched},
		{"nor is hexadecimal, which bash does not read here", "0x2", untouched},
		{"nor a number too large to hold", "99999999999999999999", untouched},
		// And the values that *are* counts, which is the same question:
		// whitespace around the digits is not part of the value, and a sign
		// is read.
		{"spaces around the digits are not part of it", `" 2 "`, "    1  b\n    2  c\n"},
		{"nor is a tab", `$'\t2'`, "    1  b\n    2  c\n"},
		{"a plus sign is read", "+2", "    1  b\n    2  c\n"},
		{"and minus zero is zero rather than negative", "-0", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, errs, code := historyStifleRun(t, historyStifleAdds(3)+"HISTSIZE="+c.value+"\nhistory\n")
			if out != c.want || errs != "" || code != 0 {
				t.Errorf("HISTSIZE=%s: out %q err %q status %d, want %q, no diagnostic and 0",
					c.value, out, errs, code, c.want)
			}
		})
	}
}

// The two routes to a two-entry list are distinguishable, and only by their
// numbers. The trim renumbers from what it dropped; entries pushed off by a
// later insert move the numbering on from where it stood.
//
// This is the row a fix that only drops entries passes and a correct one has
// to tell apart, so the assertion is the pair rather than either alone.
func TestTheTrimAndTheInsertCapNumberTheSameListDifferently(t *testing.T) {
	t.Parallel()
	trimmed, errs, code := historyStifleRun(t, historyStifleAdds(3)+"HISTSIZE=2\nhistory\n")
	if want := "    1  b\n    2  c\n"; trimmed != want || errs != "" || code != 0 {
		t.Errorf("the trim: out %q err %q status %d, want %q", trimmed, errs, code, want)
	}
	inserted, errs, code := historyStifleRun(t, historyStifleAdds(3)+"HISTSIZE=2\nhistory -s d\nhistory\n")
	if want := "    2  c\n    3  d\n"; inserted != want || errs != "" || code != 0 {
		t.Errorf("an entry after the trim: out %q err %q status %d, want %q", inserted, errs, code, want)
	}
	// Two entries either way, and the same two positions in the list.
	if strings.Count(trimmed, "\n") != strings.Count(inserted, "\n") {
		t.Fatalf("the two routes gave lists of different lengths: %q and %q", trimmed, inserted)
	}
	// The insert path on its own, with no assignment over a longer list
	// anywhere near it, which is the control: it kept its own numbers before
	// this change and still does.
	kept, _, _ := historyStifleRun(t, "HISTSIZE=2\n"+historyStifleAdds(3)+"history\n")
	if want := "    2  b\n    3  c\n"; kept != want {
		t.Errorf("the cap alone: out %q, want %q", kept, want)
	}
	grown, _, _ := historyStifleRun(t, "HISTSIZE=3\n"+historyStifleAdds(6)+"history\n")
	if want := "    4  d\n    5  e\n    6  f\n"; grown != want {
		t.Errorf("the cap over six entries: out %q, want %q", grown, want)
	}
}

// Every way of writing the assignment reaches it, because bash trims on the
// value moving and not on a particular syntax. `local` is the sharpest: the
// list is the shell's rather than the frame's, so the trim outlives the call.
func TestEveryAssignmentFormTrimsTheList(t *testing.T) {
	t.Parallel()
	const want = "    1  b\n    2  c\n"
	for _, c := range []struct{ name, line string }{
		{"plain", "HISTSIZE=2"},
		{"export", "export HISTSIZE=2"},
		{"declare", "declare HISTSIZE=2"},
		{"typeset with an attribute", "typeset -i HISTSIZE=2"},
		{"as a command's prefix", "HISTSIZE=2 true"},
		{"inside an arithmetic command", "((HISTSIZE=2))"},
		{"printf -v", "printf -v HISTSIZE 2"},
		{"in a function", "fn() { HISTSIZE=2; }\nfn"},
		{"local in a function, which outlives it", "fn() { local HISTSIZE=2; }\nfn"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, errs, code := historyStifleRun(t, historyStifleAdds(3)+c.line+"\nhistory\n")
			if out != want || errs != "" || code != 0 {
				t.Errorf("%s: out %q err %q status %d, want %q, no diagnostic and 0",
					c.line, out, errs, code, want)
			}
		})
	}
}

// Raising the size afterwards does not put back what a trim took, and does
// not disturb the numbers it left.
func TestRaisingHistsizeAfterATrimPutsNothingBack(t *testing.T) {
	t.Parallel()
	out, errs, code := historyStifleRun(t,
		historyStifleAdds(3)+"HISTSIZE=2\nHISTSIZE=10\nhistory -s d\nhistory\n")
	if want := "    1  b\n    2  c\n    3  d\n"; out != want || errs != "" || code != 0 {
		t.Errorf("out %q err %q status %d, want %q, no diagnostic and 0", out, errs, code, want)
	}
}
