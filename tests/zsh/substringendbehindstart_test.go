// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// A negative substring length whose end falls behind the offset is refused,
// and the refusal ends the shell.
//
// The sentence names the **computed end and start**, which is what makes it
// this shell's own rather than the one bash writes for the same bound: there
// the length as written is blamed and the line is given up at 1. Three rows,
// because a sentence carrying two numbers that were both wrong would still
// look right on one (#4832).
//
// Measured on zsh 5.9.2, 2026-09-27, `-f` from a script file.
func TestAnEndBehindTheOffsetIsRefused(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`v=abcdef; echo "[${v:1:-9}]"`, "substring expression: -3 < 1"},
		{`v=abcdef; echo "[${v:0:-7}]"`, "substring expression: -1 < 0"},
		{`v=abcdef; echo "[${v:3:-4}]"`, "substring expression: 2 < 3"},
	} {
		out, st := answersRun(t, tc.src+"\nprint after")
		if !strings.Contains(out, tc.want) || strings.Contains(out, "after") || st != 1 {
			t.Errorf("%s = %q (status %d), want %q and no `after`, at 1", tc.src, out, st, tc.want)
		}
	}
}

// The controls that say where the boundary is, and neither is this axis.
//
// A negative length that stays inside the value is unanimous across the panel
// and was already right here. And an end landing **exactly** at the start is
// empty at 0 — in the reference and in bash alike — so what is refused is a
// strictly smaller end and not a negative length.
func TestAnEndAtOrInsideTheStartIsNotRefused(t *testing.T) {
	out, st := answersRun(t, `v=abcdef
print "  inside=[${v:1:-2}]"
print "  atstart=[${v:1:-5}]"
print "  after=$?"`)
	want := "  inside=[bcd]\n  atstart=[]\n  after=0\n"
	if out != want || st != 0 {
		t.Errorf("the controls = %q (status %d), want %q", out, st, want)
	}
}

// The list spelling asks the same question and draws the identical sentence,
// which is measured rather than assumed — and its own control is what keeps
// it from being read as bash's rule about negative lengths on lists, since
// this shell takes one that stays inside.
func TestTheListSliceAsksTheSameBound(t *testing.T) {
	out, st := answersRun(t, `a=(p q r s t u)
print "  inside=[${a[@]:1:-2}]"
print "  behind=[${a[@]:1:-9}]"
print after`)
	if want := "  inside=[q r s]\n"; !strings.HasPrefix(out, want) {
		t.Errorf("the list control = %q, want it to start with %q", out, want)
	}
	if want := "substring expression: -3 < 1"; !strings.Contains(out, want) ||
		strings.Contains(out, "after") || st != 1 {
		t.Errorf("the list slice = %q (status %d), want %q and no `after`, at 1", out, st, want)
	}
}

// And the positional list names the bound in the numbers *it* counts in,
// which are not the ones this engine slices over: `$0` is on the front of the
// list a slice walks, and the sentence does not count it — except at offset
// 0, where it is inside the slice and it does.
//
// Five offsets, because the pair that would be got wrong by a rule keyed on
// either fact alone is offset 0 against offset 1: those two are the only ones
// whose end differs.
func TestThePositionalSliceNamesItsOwnBound(t *testing.T) {
	for _, tc := range []struct{ off, want string }{
		{"0", "substring expression: -2 < 0"},
		{"1", "substring expression: -3 < 0"},
		{"2", "substring expression: -3 < 1"},
		{"3", "substring expression: -3 < 2"},
		{"4", "substring expression: -3 < 3"},
	} {
		src := "set -- aa bb cc dd ee ff\nprint \"[${@:" + tc.off + ":-9}]\""
		out, st := answersRun(t, src)
		if !strings.Contains(out, tc.want) || st != 1 {
			t.Errorf("offset %s = %q (status %d), want %q at 1", tc.off, out, st, tc.want)
		}
	}
}
