// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"
)

// TestAJobRowIsThirtyThreeBytes: `[1]+  ` and a 27-wide state, measured byte
// for byte 2026-10-03 in the pinned image — `Running` and `Done(1)` both pad
// to the same column, and `jobs -l` narrows the state by what the id took.
func TestAJobRowIsThirtyThreeBytes(t *testing.T) {
	out, _ := runIn(t, `sleep 0.4 & jobs; jobs -l >l.txt; read -r a b c d <l.txt; echo "[$a][$c]"; wait
false & sleep 0.3; jobs`)
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %q, want three lines", out)
	}
	if want := "[1]+  Running" + strings.Repeat(" ", 20); lines[0] != want {
		t.Errorf("running row = %q, want %q", lines[0], want)
	}
	if lines[1] != "[[1]+][Running]" {
		t.Errorf("long row fields = %q, want the marker and the state", lines[1])
	}
	if want := "[1]+  Done(1)" + strings.Repeat(" ", 20); lines[2] != want {
		t.Errorf("failed row = %q, want %q", lines[2], want)
	}
}

// TestFgWithNoJobControlNamesTheSpec: the operand is read first, and a job it
// finds is refused in dash's words at 2 — `(null)` where none was written.
// Measured 2026-10-03 in the pinned image from a script with no terminal.
func TestFgWithNoJobControlNamesTheSpec(t *testing.T) {
	out, _ := run(t, `fg 2>&1; echo "rc=$?"
sleep 0.3 & fg 2>&1; echo "rc=$?"; fg %1 2>&1; echo "rc=$?"; wait`)
	for _, want := range []string{
		"fg: line 1: No current job\nrc=2\n",
		"fg: line 2: job (null) not created under job control\nrc=2\n",
		"fg: line 2: job %1 not created under job control\nrc=2\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("got %q, want %q in it", out, want)
		}
	}
}

// TestJobsPidsLetterWinsInEitherOrder: `-p` beside `-l` is the ids alone
// however the two are written.
func TestJobsPidsLetterWinsInEitherOrder(t *testing.T) {
	out, _ := runIn(t, `sleep 0.4 & p=$!; for o in -pl -lp "-p -l" "-l -p"; do jobs $o >o.txt; x=$(cat o.txt); [ "$x" = "$p" ] && echo ids || echo "listing: $x"; done; wait`)
	if want := "ids\nids\nids\nids\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
