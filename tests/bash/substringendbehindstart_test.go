// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// A negative substring length whose end falls behind the offset is refused
// here too, in a wholly different sentence: the length **as written** is
// blamed rather than the numbers it came to, and the line is given up at 1
// with the script carrying on.
//
// That difference is the reason the axis carries a wording per column — zsh
// writes `substring expression: -3 < 1` for the same bound and ends the
// shell. Measured on bash 5.3.20, 2026-09-27, and 3.2.57 says it in the same
// words (#4832).
func TestAnEndBehindTheOffsetIsRefused(t *testing.T) {
	out, st := answersRun(t, `v=abcdef
echo "[${v:1:-9}]"
echo "after st=$?"
echo second`)
	if want := "-9: substring expression < 0"; !strings.Contains(out, want) {
		t.Errorf("the refusal = %q (status %d), want it to contain %q", out, st, want)
	}
	if want := "after st=1\nsecond\n"; !strings.HasSuffix(out, want) || st != 0 {
		t.Errorf("the give-up = %q (status %d), want it to end with %q at 0", out, st, want)
	}
}

// The controls, and they are the same two the other column has: a negative
// length inside the value is `bcd` at 0, and an end landing exactly at the
// start is empty at 0.
//
// The length **as written** is what the sentence blames, which the arithmetic
// row is what says: `1-$n` is quoted back exactly as it was typed, neither
// expanded to `1-10` nor reduced to the `-9` it comes to.
func TestTheWrittenLengthIsWhatIsBlamed(t *testing.T) {
	out, st := answersRun(t, `v=abcdef
echo "inside=[${v:1:-2}]"
echo "atstart=[${v:1:-5}]"
echo "st=$?"`)
	want := "inside=[bcd]\natstart=[]\nst=0\n"
	if out != want || st != 0 {
		t.Errorf("the controls = %q (status %d), want %q", out, st, want)
	}
	out, _ = answersRun(t, "v=abcdef\nn=10\necho \"[${v:1:1-$n}]\"")
	if want := "1-$n: substring expression < 0"; !strings.Contains(out, want) {
		t.Errorf("an expression length = %q, want it to contain %q", out, want)
	}
}
