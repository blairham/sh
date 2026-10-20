// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"
)

// The `-p` letter on `export` and `readonly` is inert once operands are
// written: nothing is listed, and the operands are declared exactly as the
// same line without the letter would declare them.
//
// That is bash's and ksh93's answer and **not** dash's, which is worth a test
// of its own because dash is the shell this dialect is otherwise closest to:
// `export -p w=8` leaves `w` unset there and exports it here.
//
// Measured 2026-09-20 on BusyBox ash v1.37.0 in the pinned alpine image,
// script files under `env -i PATH=/usr/bin:/bin LC_ALL=C` with standard input
// on the null device. This shell narrowed the listing to the operand and
// declared nothing (#3904). See Semantics.ExportOrReadonlyPrintWithOperands.
func TestThePrintLetterIsInertOnceOperandsAreWritten(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, src, want string
		status          int
	}{
		{
			"a name operand lists nothing",
			`export s=5; export -p s; echo "st=$?"`,
			"st=0\n", 0,
		},
		{
			"a value operand is exported without a word",
			`export -p w=8; echo "st=$? [$w]"` + "\n" + `export -p | grep '^export w'`,
			"st=0 [8]\nexport w='8'\n", 0,
		},
		{
			// The write that follows ends the run, here as in the shell
			// this was measured against under the same invocation.
			"readonly's value operand is stored and frozen",
			`readonly -p u=9; echo "st=$? [$u]"` + "\n" + `u=2; echo "after=$?"`,
			"st=0 [9]\nash: u: is read only\n", 2,
		},
		{
			// The control that parts this column from dash: a name nothing
			// had set is *declared* by the `-p` line itself, where dash
			// declares nothing at all.
			"the control: a bare name operand is declared",
			`export -p nosuch; echo "st=$?"` + "\n" + `export -p | grep nosuch`,
			"st=0\nexport nosuch\n", 0,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, status := runIn(t, c.src)
			if out != c.want || status != c.status {
				t.Errorf("wrote %q at %d, want %q at %d", out, status, c.want, c.status)
			}
		})
	}
}

// The control that says the letter is *read* and then does nothing, rather
// than not being an option at all: an unknown letter on the same word is
// still refused, fatally, at 2.
//
// The words alone, because the line number in this shell's refusal is a
// separate and older question — BusyBox writes `line 0` for a `-c` script
// where this writes `line 1` — and a row about the print letter should not
// be the thing that pins it.
func TestAnUnknownLetterOnTheSameWordIsStillRefused(t *testing.T) {
	t.Parallel()
	out, status := runIn(t, `export -q z=1; echo "st=$?"`)
	if !strings.Contains(out, "export: ") || !strings.Contains(out, "illegal option -q") {
		t.Errorf("wrote %q, want the unknown letter refused by name", out)
	}
	if strings.Contains(out, "st=") || status != 2 {
		t.Errorf("wrote %q at %d, want the run to end at 2", out, status)
	}
}
