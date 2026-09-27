// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `hash -r` here is the whole of the call rather than the first half of one:
// an operand beside the letter is `too many arguments` at 1 and **the table
// is left standing**. Every other column in the panel clears and then hashes
// the name it was given, which is what makes this a Semantics axis rather
// than a fix — see interp.Semantics.HashClearRefusesOperands for the four
// columns (#4744).
//
// Measured 2026-09-26 on zsh 5.9.2 (`/opt/homebrew/bin/zsh`,
// aarch64-apple-darwin25.4.0), `env -i PATH=/usr/bin:/bin`, one probe at a
// time. Before this change `hash -r ls` was a silent 0 with `ls` hashed, and
// `hash -r -m 'f*'` emptied the table and then ran the listing — the
// destructive half of a command the reference declines to run.

// Every shape of operand the builtin takes, and each is the same refusal.
func TestHashClearRefusesAnOperandBesideIt(t *testing.T) {
	for _, c := range []struct{ name, src string }{
		{"a name", `hash -r ls`},
		{"an assignment", `hash -r q=/bin/ls`},
		{"a pattern", `hash -r -m 'f*'`},
		{"a named directory", `hash -d -r x`},
		{"two names", `hash -r ls cat`},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st, errs := runZshSplit(t, t.TempDir(), c.src+"\n")
			if out != "" {
				t.Errorf("stdout = %q, want nothing", out)
			}
			if st != 1 {
				t.Errorf("status = %d, want 1", st)
			}
			if want := "zsh:hash:1: too many arguments"; !strings.Contains(errs, want) {
				t.Errorf("stderr = %q, want %q in it", errs, want)
			}
		})
	}
}

// **Nothing happens.** This is the half of the bar worth having and the half
// the old reading broke: the refusal comes before the clearing, so a table
// that was full stays full. Both tables the builtin keeps are checked,
// because the named-directory one is reached by a different branch and the
// guard sits in front of both.
func TestHashClearLeavesTheTableStandingWhenItRefuses(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"the command table",
			"hash q=/bin/ls\nhash -r q2\nhash\n",
			"q=/bin/ls\n",
		},
		{
			"the command table under -m",
			"hash q=/bin/ls\nhash -r -m 'q*'\nhash\n",
			"q=/bin/ls\n",
		},
		{
			"the named-directory table",
			"hash -d foo=/tmp\nhash -d -r x\nhash -d\n",
			"foo=/tmp\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _, errs := runZshSplit(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("table afterwards = %q, want %q", out, c.want)
			}
			if !strings.Contains(errs, "too many arguments") {
				t.Errorf("stderr = %q, want the refusal in it", errs)
			}
		})
	}
}

// The control, and it is what a mutant that refused `hash -r` outright would
// fail: every column takes the letter on its own, and so does this one. The
// second row is the positive control for the first — without it, a `-r` that
// had quietly stopped clearing would pass.
func TestHashClearOnItsOwnStillEmptiesTheTable(t *testing.T) {
	out, st, errs := runZshSplit(t, t.TempDir(), "hash q=/bin/ls\nhash -r\nhash\n")
	if out != "" || st != 0 || errs != "" {
		t.Errorf("hash -r = %q status %d stderr %q, want an empty table at 0", out, st, errs)
	}
	out, st, errs = runZshSplit(t, t.TempDir(), "hash q=/bin/ls\nhash\n")
	if want := "q=/bin/ls\n"; out != want || st != 0 || errs != "" {
		t.Errorf("control: %q status %d stderr %q, want %q at 0", out, st, errs, want)
	}
}

// **Another letter is not an operand.** Measured, each of these is 0 there —
// so the refusal is keyed on a word left over after the options, not on `-r`
// standing beside anything at all.
func TestHashClearIsNotRefusedByAnotherLetter(t *testing.T) {
	for _, src := range []string{`hash -r`, `hash -r -m`, `hash -rd`, `hash -d -r`} {
		t.Run(src, func(t *testing.T) {
			out, st, errs := runZshSplit(t, t.TempDir(), src+"\n")
			if out != "" || st != 0 || errs != "" {
				t.Errorf("%s = %q status %d stderr %q, want silence at 0", src, out, st, errs)
			}
		})
	}
}
