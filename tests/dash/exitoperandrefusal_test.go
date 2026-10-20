// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"
)

// The other side of the axis: here a refused `exit` operand *does* end the
// script, because a special builtin's usage error is fatal in this shell
// (#2299).
//
// Measured on dash 0.5.12 on 2026-09-13: `exit status` says
// `exit: Illegal number: status` and the script stops at status 2. The row is
// here so that giving bash the lenient reading cannot quietly give it to this
// shell as well — the two answers come from one axis and only a measurement
// says which each shell takes.
func TestARefusedExitOperandEndsTheScript(t *testing.T) {
	out, st := runDash(t, t.TempDir(), "exit status\necho alive\n")
	if !strings.Contains(out, "Illegal number") {
		t.Errorf("said %q, want the operand refused", out)
	}
	if strings.Contains(out, "alive") {
		t.Errorf("said %q, want the script to have ended", out)
	}
	if st != 2 {
		t.Errorf("status = %d, want 2", st)
	}
}
