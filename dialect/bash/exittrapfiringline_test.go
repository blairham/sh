// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// The EXIT trap counts as having fired on the script's *first* line here, not
// on the line past its last.
//
// The question is reachable from an ordinary script rather than only from the
// sweep: a DEBUG body's lines are numbered from the line its condition fired
// on (Semantics.CommandTrapBodyLine), and a DEBUG trap fires for the commands
// of the EXIT body, so every run with both traps set asks it. With the axis
// unanswered the shell wrote `the shells disagree here and no dialect was
// chosen` onto the script's own stderr, between two lines of its output
// (#4193).
//
// Measured 2026-09-22 on bash 5.3.20, from a five-line script file and under
// `-c` alike, `env -i PATH=/usr/bin:/bin LC_ALL=C`. Inside the EXIT trap the
// two-line DEBUG body reports `1` and `2`. Counting past the end would give
// `6` and `7`, so the two answers are far apart and the reading is not an
// off-by-one.
func TestTheExitTrapFiresOnTheFirstLineAndNotPastTheLast(t *testing.T) {
	const src = "trap 'echo \"D1:$LINENO\"\necho \"D2:$LINENO\"' DEBUG\n" +
		"trap 'echo \"E:$LINENO\"' EXIT\necho x\necho y"
	out, st := answersRun(t, src)
	const want = "D1:3\nD2:4\nD1:4\nD2:5\nx\nD1:5\nD2:6\ny\nD1:1\nD2:2\nE:1"
	if got := strings.TrimSpace(out); got != want {
		t.Errorf("output\n%s\nwant\n%s", got, want)
	}
	if st != 0 {
		t.Errorf("status %d, want 0", st)
	}
	if strings.Contains(out, "no dialect was chosen") {
		t.Errorf("an unanswered axis reached a script's stderr:\n%s", out)
	}
}
