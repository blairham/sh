// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// `shift -p` takes the count off the end of the list rather than off its
// front. Measured 2026-09-26 on zsh 5.9.2 (aarch64-apple-darwin25.4.0), run
// `-f` over a script file with `env -u FPATH`.
//
// The letter is the whole of it: asked for every letter of the alphabet under
// `shift`, that shell takes `-p` and answers `bad option` to the other
// fifty-one, so this is a direction and not a facility. See
// Semantics.ShiftFromTheEndLetter, which this pins — no corpus row can, since
// the four dialects without the letter refuse the word before a direction is
// chosen.
func TestShiftFromTheEnd(t *testing.T) {
	if got := zsh.Semantics().ShiftFromTheEndLetter; got != interp.Yes {
		t.Errorf("ShiftFromTheEndLetter = %v, want Yes", got)
	}
	out, st := runZsh(t, t.TempDir(), `set -- a b c d e
shift 2;    print "A=$? [$*]"
shift -p 2; print "B=$? [$*]"
shift -p;   print "C=$? [$*]"`)
	// Row A is the control: the ordinary count form still walks the front,
	// so what `-p` changes is the direction alone.
	want := "A=0 [c d e]\nB=0 [c]\nC=0 []\n"
	if out != want || st != 0 {
		t.Errorf("shift -p = %q (status %d), want %q", out, st, want)
	}
}

// The letter reaches an array named as an operand too, and the count in front
// of it is read exactly as it is for the front form.
func TestShiftFromTheEndOfANamedArray(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `arr=(1 2 3 4)
shift -p 2 arr; print "A=$? (${(j:,:)arr})"
arr2=(1 2 3)
shift -p arr2;  print "B=$? (${(j:,:)arr2})"`)
	want := "A=0 (1,2)\nB=0 (1,2)\n"
	if out != want || st != 0 {
		t.Errorf("shift -p over an array = %q (status %d), want %q", out, st, want)
	}
}

// A count outside `0..$#` is the same complaint at the same status in either
// direction, and the list is left where it was — so `-p` is not a second
// range rule. The `-p` in the middle row is what makes this discriminating: a
// change that read the letter and then ran the front form would agree with
// the first row and not with the second.
func TestShiftFromTheEndOutOfRange(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `set -- a b c
shift -p 5; print "A=$? [$*] n=$#"
set -- a b c
shift -p -1; print "B=$? [$*] n=$#"
set -- a b c
shift -p 0; print "C=$? [$*] n=$#"`)
	want := "zsh:shift:2: shift count must be <= $#\nA=1 [a b c] n=3\n" +
		"zsh:shift:4: argument to shift must be non-negative\nB=1 [a b c] n=3\n" +
		"C=0 [a b c] n=3\n"
	if out != want || st != 0 {
		t.Errorf("shift -p out of range = %q (status %d), want %q", out, st, want)
	}
}

// Where the options end, which is the half a bare "accept the letter" would
// get wrong. `--` past the letter still ends them, a `--` in front of it
// makes `-p` an arithmetic count of zero and `2` a name operand — so the
// positional parameters are left alone — and the letters behind one `-` are
// options in their own right.
func TestShiftFromTheEndOptionScan(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `set -- a b c d e
shift -p -- 2; print "A=$? [$*]"
set -- a b c d e
shift -- -p 2; print "B=$? [$*]"
set -- a b c d e
shift -pp 2;   print "C=$? [$*]"
set -- a b c d e
shift 2 -p;    print "D=$? [$*]"`)
	want := "A=0 [a b c]\nB=0 [a b c d e]\nC=0 [a b c]\nD=0 [a b c d e]\n"
	if out != want || st != 0 {
		t.Errorf("shift -p option scan = %q (status %d), want %q", out, st, want)
	}
}
