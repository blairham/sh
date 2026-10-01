// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// **`&>` and `&>>` are two flags** (#5135): BusyBox ash and bash 3.2 read the
// first and refuse the second. See Dialect.AmpersandAppendRedirect for the
// panel.
func TestAmpersandAppendIsItsOwnFlag(t *testing.T) {
	t.Parallel()
	d := Core()
	d.AmpersandAppendRedirect = false
	if rs, _ := redirsOf(t, "echo hi &> f", d); len(rs) != 1 || rs[0].Op != TokAmpGreat {
		t.Errorf("`&>` without the append flag: %v, want the one operator", rs)
	}
	if _, err := Parse("echo hi &>> f", d); err == nil {
		t.Error("`&>>` parsed without the flag that gives it; want a refusal")
	}
	d.ClobberOverrideMarker = true
	if _, err := Parse("echo hi &>>| f", d); err == nil {
		t.Error("`&>>|` parsed without the operator it is a marker on")
	}
	// The control: both flags, and the operator is read as itself.
	if rs, _ := redirsOf(t, "echo hi &>> f", Core()); len(rs) != 1 || rs[0].Op != TokAmpDGreat {
		t.Errorf("`&>>` in the core: %v, want the one operator", rs)
	}
}
