// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// What HISTIGNORE and HISTCONTROL do to an entry `history -s` was handed.
//
// Measured 2026-09-21 against bash 5.3.20 at `/opt/homebrew/bin/bash` — the
// panel's bash, not `/bin/bash`, which is 3.2 — under `env -i` with a scratch
// HOME (#4057). Every want below is the byte-for-byte listing the real binary
// printed for the same script, run side by side.
//
// `-s` went straight into the list and read neither variable, while the
// reader's own lines went through both. One route carrying a rule the other
// omits is the shape this tree keeps finding, and it is the same question
// either way: whether this entry joins the list.

// historyIgnoreSilent quiets the reader, so the only entries a case leaves
// are the ones `history -s` stored.
//
// **The silencer is the feature under test**, which is worth saying out loud:
// #4031's first grid was wrong because the reading command was itself an
// entry and moved the numbers under it. HISTIGNORE is the tool for stopping
// that and is also half of what is being measured here, so the pattern that
// quiets the instrument is set *before* `set -o history` — the line carrying
// the assignment is recorded before the assignment takes effect, so a rule
// written after the list is on cannot hide its own line. Cases that need
// their own patterns pass them here and get the quiet ones as well.
func historyIgnoreSilent(extra string) string {
	patterns := "history*:HISTCONTROL*:HISTSIZE*:printf*"
	if extra != "" {
		patterns += ":" + extra
	}
	return "HISTIGNORE='" + patterns + "'\nset -o history\n"
}

// historyIgnoreRun runs a script under those ignores and answers the listing,
// the diagnostics and the status.
//
// The whole listing, entry numbers included: a containment check cannot see
// an entry that should have been dropped but arrived under a different
// number, and renumbering is exactly where the erasedups rows below differ
// from every other way a list gets shorter.
func historyIgnoreRun(t *testing.T, extra, src string) (string, string, int) {
	t.Helper()
	return historyRun(t, historyIgnoreSilent(extra)+src)
}

// The issue's own three rows, and the corners around them.
func TestHistoryStoreReadsHistignoreAndHistcontrol(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, ignore, src, want string }{
		{
			"a pattern keeps the operand out",
			"secret*",
			"history -s secretline\nhistory -s keepme\nhistory\n",
			"    1  keepme\n",
		},
		{
			// Anchored at both ends, so it is the entry and not a search
			// inside it: `echo` drops `echo` and keeps `echo one`. A shell
			// matching a prefix would leave nothing.
			"a pattern is the whole entry",
			"echo",
			"history -s echo\nhistory -s 'echo one'\nhistory\n",
			"    1  echo one\n",
		},
		{
			// `&` is documented as the previous history line, and for `-s`
			// there is no line — so it is the entry the list already ends
			// with, which is a well-defined thing even here. Measured: `x`,
			// `x`, `y`, `y` leaves one of each.
			"the & pattern is the entry before it",
			"&",
			"history -s x\nhistory -s x\nhistory -s y\nhistory -s y\nhistory\n",
			"    1  x\n    2  y\n",
		},
		{
			// Not a typed line, and the rule applies anyway. Worth pinning
			// rather than reasoning about: `ignorespace` reads as a gesture
			// somebody makes at a keyboard, and bash applies it to an
			// operand a script wrote.
			"a leading blank hides an operand",
			"",
			"HISTCONTROL=ignorespace\nhistory -s ' hidden'\nhistory -s kept\nhistory\n",
			"    1  kept\n",
		},
		{
			"ignoredups is the entry immediately before",
			"",
			"HISTCONTROL=ignoredups\nhistory -s one\nhistory -s one\nhistory -s two\nhistory\n",
			"    1  one\n    2  two\n",
		},
		{
			// A word that is none of the four is simply not one of them.
			// Neither an error nor a poisoned value: the rule the value does
			// name still applies.
			"an unknown word in HISTCONTROL leaves the rest standing",
			"",
			"HISTCONTROL=bogus:ignoredups:alsobogus\nhistory -s d\nhistory -s d\nhistory -s e\nhistory\n",
			"    1  d\n    2  e\n",
		},
		{
			"ignoreboth is both of them",
			"",
			"HISTCONTROL=ignoreboth\nhistory -s ' q'\nhistory -s r\nhistory -s r\nhistory\n",
			"    1  r\n",
		},
		{
			// Either rejecting is enough, and neither needs the other: the
			// first entry is refused twice over, the second by the pattern
			// alone, the third by the blank alone.
			"the two rules do not need to agree",
			"zap*",
			"HISTCONTROL=ignorespace\nhistory -s ' zapper'\nhistory -s zapper\n" +
				"history -s ' plain'\nhistory -s plain\nhistory\n",
			"    1  plain\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, errs, st := historyIgnoreRun(t, c.ignore, c.src)
			if out != c.want || errs != "" || st != 0 {
				t.Errorf("out = %q errs = %q status = %d, want %q at 0", out, errs, st, c.want)
			}
		})
	}
}

// erasedups takes out every earlier copy, and what is left is renumbered from
// the front.
//
// The numbers are the half worth being careful about, because two different
// things shorten a list and they number what survives differently. HISTSIZE
// dropping an entry off the front moves the numbering on — the oldest entry
// kept is numbered by how many were dropped — and erasing a copy from the
// middle does not. The last row holds both at once and is the discriminating
// one: with `HISTSIZE=3` the list reads `2 c`, `3 d`, `4 b`, so the single
// entry HISTSIZE pushed off still counts and the copy erasedups removed does
// not.
func TestHistoryStoreErasedupsRenumbersFromTheFront(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{
			"the earlier copy goes and the new one stays at the end",
			"HISTCONTROL=erasedups\nhistory -s a\nhistory -s b\nhistory -s a\nhistory\n",
			"    1  b\n    2  a\n",
		},
		{
			"every earlier copy, not just the last",
			"HISTCONTROL=erasedups\nhistory -s a\nhistory -s b\nhistory -s a\n" +
				"history -s c\nhistory -s a\nhistory\n",
			"    1  b\n    2  c\n    3  a\n",
		},
		{
			"an entry HISTSIZE dropped still counts and an erased copy does not",
			"HISTCONTROL=erasedups\nHISTSIZE=3\nhistory -s a\nhistory -s b\n" +
				"history -s c\nhistory -s d\nhistory -s b\nhistory\n",
			"    2  c\n    3  d\n    4  b\n",
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

// The builtin's own line is dropped whether or not the operand then joins the
// list, and the entry a duplicate is measured against is what that leaves.
//
// Both halves need a list that already holds something, which is why the
// reader is left talking here rather than silenced: with nothing in front of
// it, `-s` has no own line to drop and nothing to be a duplicate of, and
// either bug would pass.
func TestHistoryStoreDropsItsOwnLineEvenWhenTheOperandIsIgnored(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{
			// `alpha` survives: the `history -s secretline` line was
			// recorded by the reader, dropped by the builtin, and then
			// nothing replaced it.
			"an ignored operand leaves the entries before it alone",
			"set -o history\nhistory -s alpha\nHISTIGNORE='secret*'\n" +
				"history -s secretline\nhistory\n",
			"    1  alpha\n    2  HISTIGNORE='secret*'\n    3  history\n",
		},
		{
			// Not a duplicate: the own line has already gone, so what stands
			// before the second `alpha` is the assignment and not the first
			// `alpha`. A shell comparing against the line it was written on,
			// or against the newest entry before the drop, records nothing
			// here.
			"a duplicate is measured against what the drop leaves",
			"set -o history\nhistory -s alpha\nHISTCONTROL=ignoredups\n" +
				"history -s alpha\nhistory\n",
			"    1  alpha\n    2  HISTCONTROL=ignoredups\n    3  alpha\n    4  history\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, errs, st := historyRun(t, c.src)
			if out != c.want || errs != "" || st != 0 {
				t.Errorf("out = %q errs = %q status = %d, want %q at 0", out, errs, st, c.want)
			}
		})
	}
}

// A file read back is not an entry being offered, and none of the rules touch
// it.
//
// Measured, and it is the third answer the issue asked for rather than the
// same one: `-r` loads a line the pattern matches, and loads both copies of a
// line under `ignoredups:erasedups`. So the gate belongs on the two routes a
// command arrives by and not on historyLoad — a shell that put it on
// everything would lose lines out of somebody's file on the way in.
func TestHistoryFileReadIgnoresNeitherVariable(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, ignore, src, want string }{
		{
			"a pattern does not filter a file",
			"secret*",
			"printf 'fromfile1\\nsecretfile\\nfromfile2\\n' > hf\nhistory -r hf\nhistory\n",
			"    1  fromfile1\n    2  secretfile\n    3  fromfile2\n",
		},
		{
			"neither does either duplicate rule",
			"",
			"HISTCONTROL=ignoredups:erasedups\n" +
				"printf 'dup\\ndup\\nother\\n' > hf\nhistory -r hf\nhistory\n",
			"    1  dup\n    2  dup\n    3  other\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			// In a directory of its own, because the script writes the file
			// it then reads: historyRun runs where the test process stands.
			out, st := runBash(t, t.TempDir(), historyIgnoreSilent(c.ignore)+c.src)
			if out != c.want || st != 0 {
				t.Errorf("out = %q status = %d, want %q at 0", out, st, c.want)
			}
		})
	}
}
