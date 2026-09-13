// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// A refused `exit` operand does not end the script here (#2299).
//
// Measured against bash 5.3.15 on 2026-09-13, from a script file and from
// `-c` alike: `exit status` reports `exit: status: numeric argument required`,
// leaves 2 in `$?` and runs the next command. Reading `exit` as "the builtin
// that leaves, whatever happened" ended the script instead, which is the
// answer dash and this same bash called `sh` give — a different column.
//
// It cost a whole file of bash's own suite, which ran to the end under the
// reference and stopped here at a usage error nobody was asked about.
func TestARefusedExitOperandLeavesTwoAndCarriesOn(t *testing.T) {
	out, st := runBash(t, t.TempDir(), "exit status\necho \"st=$?\"\necho alive\n")
	if !strings.Contains(out, "numeric argument required") {
		t.Errorf("said %q, want the operand refused", out)
	}
	if !strings.Contains(out, "st=2") {
		t.Errorf("said %q, want st=2 left behind", out)
	}
	if !strings.Contains(out, "alive") {
		t.Errorf("said %q, want the script to carry on", out)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// And the control, so the row above cannot read as "`exit` no longer exits":
// an operand this shell takes still ends the script with it.
func TestAGoodExitOperandStillEndsTheScript(t *testing.T) {
	out, st := runBash(t, t.TempDir(), "exit 3\necho alive\n")
	if strings.Contains(out, "alive") {
		t.Errorf("said %q, want the script to have ended", out)
	}
	if st != 3 {
		t.Errorf("status = %d, want 3", st)
	}
}
