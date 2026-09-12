// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"
)

// A dash *inside* an option bundle is a letter `print` accepts and does
// nothing with, which is not the rule the lone `-` word follows (#1708).
//
// Measured 2026-09-12 on zsh 5.9.2. The two readings are told apart by the
// two rows marked below, and neither alone can do it:
//
//   - `print - -n x` prints `-n x`, so the lone word really ended the options
//     and `-n` after it is an operand. An inert letter would have left `-n`
//     an option and printed `x` with no newline.
//   - `print -n-r x` prints `x` with no newline, so `-n` and `-r` both
//     applied around a dash that stopped nothing. An end-of-options reading
//     would have made `-r x` two operands.
//
// `print -` and `print - x` agree under both readings and are here as the
// rows that cannot decide anything, kept so the discriminating pair is not
// mistaken for the whole measurement.
func TestPrintReadsADashInsideABundleAsAnInertLetter(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// Discriminating: the lone word ends the options.
		{`print - -n x`, "-n x\n"},
		// Discriminating: a dash in a bundle does not.
		{`print -n-r x`, "x"},

		{`print -n- x`, "x"},
		{`print --n x`, "x"},
		{`print ---`, "\n"},
		{`print -rn- x`, "x"},

		// Neither reading is decided by these two.
		{`print -`, "\n"},
		{`print - x`, "x\n"},

		// `--` is still the end-of-options word, and a dash with a digit
		// after it is still an operand (#1652).
		{`print -- - x`, "- x\n"},
		{`print -1`, "-1\n"},
		// After `-R` the parsing is echo's, where a word that is not made of
		// `e` and `n` is an operand — a dash in it included.
		{`print -R --n x`, "--n x\n"},
	} {
		out, st := runZsh(t, t.TempDir(), tc.src)
		if st != 0 || out != tc.want {
			t.Errorf("%s: out %q status %d, want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// The character the refusal names, which is what the bug was about: a heading
// written `print "--- x ---"` is a bad option in this shell too, and it names
// the space after the two dashes rather than a dash. Naming the wrong
// character sends the reader to the wrong part of the line.
func TestPrintNamesTheCharacterItActuallyStoppedOn(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`print "--- x ---"`, "zsh:print:1: bad option: - \n"},
		{`print -n-Q x`, "zsh:print:1: bad option: -Q\n"},
		{`print -Q x`, "zsh:print:1: bad option: -Q\n"},
	} {
		out, st := runZsh(t, t.TempDir(), tc.src)
		if st != 1 || out != tc.want {
			t.Errorf("%s: out %q status %d, want %q at 1", tc.src, out, st, tc.want)
		}
	}
}
