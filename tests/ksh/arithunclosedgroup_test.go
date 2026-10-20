// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// A parenthesised group that could not be closed is an arithmetic failure
// here and not a parse one, and the difference is the status a caller reads.
// This shell does end the script over a failed `(( ))` — that is
// ArithCommandErrorIsFatal — but it ends it at the arithmetic status, so a
// script that traps on 1 sees 1.
//
// Measured 2026-09-17 against ksh93u+ 2012-08-01, a script file, `env -i`
// with LC_ALL=C: `echo B; (( (echo a) )); echo A` writes `B`, then
// ` (echo a) : arithmetic syntax error`, and leaves 1 with `A` unwritten.
// Ours read it as a parse failure and left 3, which a caller cannot tell from
// an unmatched quote (#3071).
func TestAnUnclosedGroupIsAnArithmeticFailureAndNotAParseOne(t *testing.T) {
	out, st := answersRun(t, "echo B\n(( (echo a) ))\necho A\n")
	if want := "(echo a) : arithmetic syntax error"; !strings.Contains(out, want) {
		t.Errorf("got %q, want it to contain %q", out, want)
	}
	if st != 1 {
		t.Errorf("status %d, want 1", st)
	}
	if !strings.Contains(out, "B") || strings.Contains(out, "A\n") {
		t.Errorf("got %q, want B written and A not", out)
	}
}
