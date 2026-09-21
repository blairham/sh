// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// Which builtin on a recorded line takes that line out of the list.
//
// `history -s` is documented to remove "the last command in the history
// list" before adding its arguments, and `history -p` is measured to do the
// same. Both are about the **one line the builtin is written on**, so a line
// carrying several of them asks a question neither wording answers: does the
// second call take the entry the first one left, or has the line already
// gone?
//
// Measured 2026-09-21 against bash 5.3.20 at `/opt/homebrew/bin/bash` — the
// panel's bash, not `/bin/bash`, which is 3.2 — under `env -i` with a
// scratch HOME (#4072). Every want below is the byte-for-byte listing the
// real binary printed for the same script.
//
// The answer is not one rule: **`-s` takes the line and `-p` only reads
// it.** Three `-s` on a line leave three entries, while two `-p` on a line
// take two — the second `-p` eating the entry that was there before the
// line. So the mark is consumed by `-s` and left standing by `-p`, and a
// shell clearing it in the shared step would get the `-p` rows wrong in the
// other direction.
//
// The cases below run through historyIgnoreRun, whose HISTIGNORE keeps the
// reader's other lines out of the list; the line under test starts with
// `true` or `for` precisely so that it is *not* kept out.
func TestHistoryOwnLineIsTakenOnceByStoreAndEveryTimeByPrint(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{
			// The issue's own row. A shell taking the line again would
			// answer `1  c` alone: each later `-s` would eat the entry the
			// one before it added, and the number staying at 1 says a
			// removal rather than a trim.
			"three -s on one line leave three entries",
			"true; history -s a; history -s b; history -s c\nhistory\n",
			"    1  a\n    2  b\n    3  c\n",
		},
		{
			// And the line is still taken once: `pre` survives, so the
			// entry that went is the `true; …` line rather than the one
			// under it.
			"the first -s still takes the line",
			"history -s pre\ntrue; history -s a; history -s b; history -s c\nhistory\n",
			"    1  pre\n    2  a\n    3  b\n    4  c\n",
		},
		{
			// Same shape without a `;` in sight, which is what says this is
			// about the recorded line and not about the separator: a loop
			// body runs the builtin three times on one line too.
			"a loop body is one line as well",
			"for i in 1 2 3; do history -s \"e$i\"; done\nhistory\n",
			"    1  e1\n    2  e2\n    3  e3\n",
		},
		{
			// `-p` first: it takes the line, and the `-s` after it takes
			// `pre`, so `-p` left the mark standing. This is the row that
			// refuses "clear the mark wherever it is read".
			"an -s after a -p takes the entry before it",
			"history -s pre\ntrue; history -p pp; history -s b\nhistory\n",
			"pp\n    1  b\n",
		},
		{
			"two -p on one line take two entries",
			"history -s pre1; history -s pre2\ntrue; history -p x; history -p y\nhistory\n",
			"x\ny\n    1  pre1\n",
		},
		{
			// The other direction of the same asymmetry: once `-s` has
			// taken the line, a `-p` behind it on that line finds nothing
			// marked and leaves `a` alone.
			"an -s clears the mark for a later -p",
			"history -s pre\ntrue; history -s a; history -p x\nhistory\n",
			"x\n    1  pre\n    2  a\n",
		},
		{
			// Marked with nothing left to take: the `-s` does not fall
			// through and store its operand, and it does not consume the
			// mark either, so the `-s` behind it stores nothing too.
			"an -s with nothing left to take stores nothing",
			"true; history -p x; history -s b; history -s c\nhistory\n",
			"x\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, errs, st := historyIgnoreRun(t, "", c.src)
			if out != c.want || errs != "" || st != 0 {
				t.Errorf("out = %q errs = %q status = %d, want %q at 0", out, errs, st, c.want)
			}
		})
	}
}

// And the status the two letters answer when the mark is set and the list is
// empty, which is where they part a second time: `-p` fails and prints
// nothing for the operand it never expanded, while `-s` is quietly 0.
func TestHistoryOwnLineMissingStatus(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, src, want string
		status          int
	}{
		{"-p fails", "true; history -p x; history -p y\n", "x\n", 1},
		{"-s is silent", "true; history -p x; history -s b\n", "x\n", 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, errs, st := historyIgnoreRun(t, "", c.src)
			if out != c.want || errs != "" || st != c.status {
				t.Errorf("out = %q errs = %q status = %d, want %q at %d", out, errs, st, c.want, c.status)
			}
		})
	}
}
