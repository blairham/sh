// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// `[n,m]` is the glob qualifier that selects by **position in the match
// list**, and it is not a file attribute at all.
//
// It was `unknown file attribute: [` at 1 — the qualifier reader reaching the
// bracket looking for a letter — which is the first failing chunk of
// `D05array.ztst` (#4968).
//
// Measured 2026-09-28 against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `go version -m` says *not a Go executable* for
// it — under `-f` from a script file with `env -i PATH=/usr/bin:/bin
// TERM=dumb` and a scratch `HOME`, in a directory holding `a b c d e`.
func TestTheGlobPositionRange(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"a", "b", "c", "d", "e"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ name, src, want string }{
		{"the n-th through the m-th", `print -r -- *([2,4])`, "b c d\n"},
		{"one number is one name", `print -r -- *([1])`, "a\n"},
		{"and it counts from one", `print -r -- *([3])`, "c\n"},
		{"a negative end counts from the last", `print -r -- *([2,-1])`, "b c d e\n"},
		{"and so does a negative start", `print -r -- *([-2,-1])`, "d e\n"},
		{"a lone negative is one name from the end", `print -r -- *([-1])`, "e\n"},
		{
			// The two ends are **not** symmetrical, which is two rows: an
			// end past the last name is clamped and a start past it is not.
			"an end past the last name is the last name",
			`print -r -- *([2,99])`, "b c d e\n",
		},
		{
			// A range that selects nothing is **not** "no matches": the word
			// is deleted and the status is 0, where a pattern that matches
			// nothing ends the script. The row below is the control.
			"a start past the last name selects nothing, at 0",
			`print -r -- *([9]); print -r -- "st=$?"`, "\nst=0\n",
		},
		{"and so does position zero", `print -r -- *([0]); print -r -- "st=$?"`, "\nst=0\n"},
		{"and so does an end in front of its start", `print -r -- *([4,2]); print -r -- "st=$?"`, "\nst=0\n"},
		{
			// The numbers are **arithmetic**, which is what explains this:
			// `x` is a name, an unset name is zero, and zero is no position.
			// A rule that read digits would have complained here.
			"the numbers are arithmetic, so an unset name is no position",
			`print -r -- *([x]); print -r -- "st=$?"`, "\nst=0\n",
		},
		{
			"and a name that holds a number is that position",
			`i=2; print -r -- *([i])`, "b\n",
		},
		{
			"and an expression is evaluated",
			`print -r -- *([1+1,2*2])`, "b c d\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZshPrelude(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
	// The control that says an empty *range* is not an empty *match*: the
	// same directory, a pattern that matches nothing, and the ordinary
	// refusal. Without this the rows above would pass against a shell that
	// had stopped refusing unmatched patterns entirely.
	t.Run("a pattern that matches nothing is still refused", func(t *testing.T) {
		out, st := runZshPrelude(t, dir, `print -r -- zzz*([1])`+"\n"+`print -r -- unreached`)
		if want := "zsh:1: no matches found: zzz*([1])\n"; out != want || st == 0 {
			t.Errorf("an unmatched pattern = %q (status %d), want %q and a failure", out, st, want)
		}
	})
}

// The range is taken **after** everything else, sort included.
//
// Its own test because it needs a directory with two kinds of entry in it, and
// because the two rows that pin it would each pass alone under the wrong rule:
// a range applied before the file tests gives the same answer whenever the
// tests keep a prefix of the list.
func TestTheGlobPositionRangeIsTakenLast(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"f1", "f2", "f3"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, n := range []string{"dir1", "dir2"} {
		if err := os.Mkdir(filepath.Join(dir, n), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ name, src, want string }{
		{
			// Sorted, the five are `dir1 dir2 f1 f2 f3`, so a range taken
			// first would keep the two directories and the `.` would then
			// leave nothing.
			"the file test narrows before the range is taken",
			`print -r -- *([1,3].)`, "f1 f2 f3\n",
		},
		{
			"wherever the range is written in the list",
			`print -r -- *(.[1,2])`, "f1 f2\n",
		},
		{
			// And the control on the other side: with no file test, the
			// same positions are the directories.
			"and with no test the same positions are the other names",
			`print -r -- *([1,3])`, "dir1 dir2 f1\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZshPrelude(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// The two malformed spellings, which are the arithmetic reader's own
// sentences rather than the qualifier reader's.
func TestTheGlobPositionRangeRefusals(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, src, want string }{
		{
			// An empty half is the same complaint `${a[1,]}` gives, through
			// the same function rather than a second copy of the wording.
			"an empty end",
			`print -r -- *([1,])`, "zsh:1: bad math expression: empty string\n",
		},
		{"an empty start", `print -r -- *([,2])`, "zsh:1: bad math expression: empty string\n"},
		{
			// A bracket that never closes is a subscript that never closed,
			// not a file attribute this shell does not know.
			"and a bracket that never closes",
			`print -r -- *([1)`, "zsh:1: invalid subscript\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZshPrelude(t, dir, tc.src+"\nprint -r -- unreached")
			if out != tc.want || st == 0 {
				t.Errorf("%s = %q (status %d), want %q and a failure", tc.src, out, st, tc.want)
			}
		})
	}
}
