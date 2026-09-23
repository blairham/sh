// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// The `time` keyword here reads two operands the core does not, and a
// repetition of itself folds rather than nesting. All three were measured
// 2026-09-23 on bash 5.3.20 and confirmed on 5.3.15 with `TIMEFORMAT` set to
// something the POSIX layout cannot produce, which is what tells the two
// reports apart:
//
//	$ TIMEFORMAT=MARK bash -c 'time time echo a'   a, then one MARK
//	$ TIMEFORMAT=MARK bash -c 'time time -p echo a'
//	                                               a, then real/user/sys
//	$ TIMEFORMAT=MARK bash -c 'time -- echo a'     a, then real/user/sys
//	$ TIMEFORMAT=MARK bash -c 'time -- -p echo a'  -p: command not found
//
// zsh answers the third with `command not found: --`, and ksh93 consumes the
// `--` but answers in a third layout of its own, so neither is evidence for
// the rule as written here (#4161).

// countReports says how many of each report the output holds: the marker a
// TIMEFORMAT wrote, and the POSIX layout's own `real` line.
func countReports(out string) (marks, posix int) {
	for line := range strings.SplitSeq(out, "\n") {
		switch {
		case strings.TrimSpace(line) == "MARK":
			marks++
		case strings.HasPrefix(line, "real"):
			posix++
		}
	}
	return marks, posix
}

// TestARepeatedTimeKeywordReportsOnceHere is the fold. Asserting only that
// the command ran would pass against a nested clause, so the count of reports
// is the assertion, and the `-p` row is what says the inner keyword's flag
// reached the one report rather than being dropped with it.
func TestARepeatedTimeKeywordReportsOnceHere(t *testing.T) {
	for _, tc := range []struct {
		src          string
		marks, posix int
	}{
		{`TIMEFORMAT=MARK; time echo a`, 1, 0},
		{`TIMEFORMAT=MARK; time time echo a`, 1, 0},
		{`TIMEFORMAT=MARK; time time time echo a`, 1, 0},
		// The inner `-p` reaches the single report.
		{`TIMEFORMAT=MARK; time time -p echo a`, 0, 1},
		{`TIMEFORMAT=MARK; time -p time echo a`, 0, 1},
	} {
		out, st := answersRun(t, tc.src)
		if st != 0 {
			t.Errorf("%s: status %d: %s", tc.src, st, out)
			continue
		}
		if !strings.Contains(out, "\na\n") && !strings.HasPrefix(out, "a\n") {
			t.Errorf("%s: the timed command did not run: %q", tc.src, out)
		}
		if marks, posix := countReports(out); marks != tc.marks || posix != tc.posix {
			t.Errorf("%s: %d marker and %d POSIX reports, want %d and %d: %q",
				tc.src, marks, posix, tc.marks, tc.posix, out)
		}
	}
}

// TestTimeReadsOneDoubleDashHere: the `--` is consumed *and* selects the
// POSIX layout on its own, which a test that only checked the command ran
// would not see. One only — a second `--`, and a `-p` written after it, are
// the command.
func TestTimeReadsOneDoubleDashHere(t *testing.T) {
	out, st := answersRun(t, `TIMEFORMAT=MARK; time -- echo a`)
	if st != 0 {
		t.Fatalf("status %d: %s", st, out)
	}
	if marks, posix := countReports(out); marks != 0 || posix != 1 {
		t.Errorf("`time -- echo a`: %d marker and %d POSIX reports, want 0 and 1: %q", marks, posix, out)
	}

	for _, src := range []string{`time -- -p echo a`, `time -p -- -- echo a`} {
		out, st := answersRun(t, src)
		if st == 0 {
			t.Errorf("%s: status 0 — the second operand should have been the command: %q", src, out)
		}
		if !strings.Contains(out, "command not found") {
			t.Errorf("%s: want a command-not-found for the second operand, got %q", src, out)
		}
	}
}

// TestABareTimeBodyKeepsItsTrailingSpaceHere: `type` writes a function's body
// back from the tree, and bash writes the separator after a `time` that has
// no pipeline. Measured through `cat -A`, the body line is `····time·`.
func TestABareTimeBodyKeepsItsTrailingSpaceHere(t *testing.T) {
	out, st := answersRun(t, "f() { time; }\ntype f")
	if st != 0 {
		t.Fatalf("status %d: %s", st, out)
	}
	// Whole line, so the trailing space is part of what is asserted — a
	// Contains on "time" cannot see it.
	if !strings.Contains(out, "\n    time \n") {
		t.Errorf("body line is not %q: %q", "    time ", out)
	}
	// And a `time` with a pipeline takes no second separator.
	out, st = answersRun(t, "g() { time echo a; }\ntype g")
	if st != 0 {
		t.Fatalf("status %d: %s", st, out)
	}
	if !strings.Contains(out, "\n    time echo a\n") {
		t.Errorf("body line is not %q: %q", "    time echo a", out)
	}
}
