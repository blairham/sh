// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"
)

// `setopt numericglobsort` sorts a pathname expansion's matches on the
// numbers their names hold, so `f2` comes before `f10`.
//
// Measured on zsh 5.9.2 (aarch64-apple-darwin25.4.0), `-f`, 2026-09-26, in a
// directory holding the names each row names. The option *off* is the control
// on every row and already agreed before this was wired (#4555).
func TestNumericGlobSortOrdersOnTheNumbersInTheNames(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"f1", "f2", "f10", "b2x", "b10x"} {
		writeFile(t, dir, f, "")
	}
	out, st := runZsh(t, dir,
		"setopt numericglobsort\nprint -r -- set: *\n"+
			"unsetopt numericglobsort\nprint -r -- unset: *\n")
	want := "set: b2x b10x f1 f2 f10\nunset: b10x b2x f1 f10 f2\n"
	if st != 0 || out != want {
		t.Errorf("out %q status %d, want %q", out, st, want)
	}
}

// It reaches filename generation and nothing else: the flag that asks for
// this order at an expansion is `n` and is written there, so `${(o)a}` is the
// plain order in both states of the option.
func TestNumericGlobSortDoesNotReachTheOrderFlags(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		"a=(f10 f2 f1)\nsetopt numericglobsort\nprint -r -- ${(o)a}\n"+
			"unsetopt numericglobsort\nprint -r -- ${(o)a}\n")
	if want := "f1 f10 f2\nf1 f10 f2\n"; st != 0 || out != want {
		t.Errorf("out %q status %d, want %q", out, st, want)
	}
}

// And the state is reported back on the three surfaces a script reads, which
// is the half that was already right and must stay so.
func TestNumericGlobSortIsReportedBackInBothStates(t *testing.T) {
	const src = `[[ -o numericglobsort ]] && print a=on || print a=off
setopt numericglobsort
[[ -o numericglobsort ]] && print b=on || print b=off
print c=${options[numericglobsort]}`
	out, st := runZsh(t, t.TempDir(), src)
	if want := "a=off\nb=on\nc=on\n"; st != 0 || out != want {
		t.Errorf("out %q status %d, want %q", out, st, want)
	}
}
