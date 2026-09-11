// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A word whose first character after the dash is a **digit** is an operand to
// `print`, not an option word. Measured 2026-09-10 against zsh 5.9.2 with no
// startup files, where every line below prints itself:
//
//	print -1        -1
//	print -12       -12
//	print -1 -2     -1 -2
//	print -r -1     -1
//	print -1x       -1x
//	print -0        -0
//	print -1.5      -1.5
//
// So it is the first character that decides and not the word being a number:
// `-1x` and `-1.5` are no more numbers than `-r` is, and they print. The rule
// is one-sided — a digit that is *not* first is still an option letter, and
// `print -n1` is `bad option: -1` in that shell as it is here.
//
// Before this, anything printing a negative number without `--` in front of it
// drew a complaint about an option nobody wrote, which is how it was found:
// `print $(( systell(99) ))` answers -1 for a descriptor nothing is open at,
// and the honest answer became a diagnostic at the point of printing it
// (#1652).
func TestADashBeforeADigitIsAnOperandToPrint(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"one digit", "print -1", "-1\n"},
		{"two digits", "print -12", "-12\n"},
		{"two operands", "print -1 -2", "-1 -2\n"},
		{"after an option word", "print -r -1", "-1\n"},
		{"a digit then a letter", "print -1x", "-1x\n"},
		{"zero", "print -0", "-0\n"},
		{"not an integer", "print -1.5", "-1.5\n"},
		// The word ends the options where it stands rather than being
		// stepped over: the `-r` after it prints as text.
		{"the words after it are operands too", "print -1 -r", "-1 -r\n"},
		// A letter word still bundles, and the operand still arrives.
		{"a letter word before it still applies", "print -l -1 -2", "-1\n-2\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), c.src)
			if out != c.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", c.src, out, st, c.want)
			}
		})
	}
}

// The other side of the same rule, and the half that must not move: a digit
// that is not the first character after the dash is an option letter, and an
// option letter `print` does not have is refused by name. Measured in that
// shell as `zsh:print:1: bad option: -1` at status 1 for both spellings.
func TestADigitAfterALetterIsStillAnOptionLetterToPrint(t *testing.T) {
	for _, src := range []string{"print -n1", "print -r1"} {
		out, st, errs := runZshSplit(t, t.TempDir(), src)
		if out != "" || st != 1 {
			t.Errorf("%s = %q (status %d), want nothing at 1", src, out, st)
		}
		if want := "bad option: -1\n"; !hasSuffixLine(errs, want) {
			t.Errorf("%s wrote %q, want it to end with %q", src, errs, want)
		}
	}
}

// hasSuffixLine reports whether the diagnostic ends with the given sentence,
// the shell's own location in front of it left to the caller's eye.
func hasSuffixLine(errs, want string) bool {
	return len(errs) >= len(want) && errs[len(errs)-len(want):] == want
}
