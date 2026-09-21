// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package oracle

import (
	"strings"
	"testing"
)

// A shell asked to be interactive with no terminal names the process group it
// could not hand the terminal to, and it writes **two** shapes of number
// there.
//
// Measured 2026-09-21, bash 5.3.20 at /opt/homebrew/bin/bash: the same binary
// with the same command string, started once into a process group of its own
// and once into its caller's, wrote `(-1)` and `(78796)`. Which of the two a
// case sees is a fact about how the runner started it and not about the
// shell, so both have to reach the record as the same cell — a mask over the
// digits alone would record one arrangement as `(N)` and the other as `(-1)`,
// and a record that moves with how it was collected cannot tell a shell that
// changed from a shell that was run differently (#4012).
func TestBothProcessGroupSpellingsReachTheRecordAlike(t *testing.T) {
	sh := Found{Shell: Shell{Name: "bash"}, Path: "/opt/homebrew/bin/bash"}
	dir := "/var/folders/xy/oracle-123"

	const line = "bash: cannot set terminal process group (%s): Inappropriate ioctl for device\n"
	inherited := normalize(strings.Replace(line, "%s", "78796", 1), sh, dir)
	leader := normalize(strings.Replace(line, "%s", "-1", 1), sh, dir)

	if inherited != leader {
		t.Errorf("the two spellings reached the record apart:\n  %q\n  %q", inherited, leader)
	}
	if want := "process group (N)"; !strings.Contains(leader, want) {
		t.Errorf("normalized to %q, want it to contain %q", leader, want)
	}
	// And masked rather than dropped: *that* the shell named a group is part
	// of the complaint, so a record with the parentheses emptied out would
	// have lost the half of the line that is a fact about the shell.
	if strings.Contains(leader, "process group ()") {
		t.Errorf("normalized to %q, which drops the group rather than masking it", leader)
	}
}

// And the mask stays anchored on the role. A bare `-1` elsewhere in a line is
// a number a shell meant, and a mask that reached it would spend the
// determinism it was added to buy — the failure #3988 records, where an
// unanchored mask over `$$` ate a line number that happened to equal the pid.
func TestTheProcessGroupMaskDoesNotReachOtherNumbers(t *testing.T) {
	sh := Found{Shell: Shell{Name: "bash"}, Path: "/opt/homebrew/bin/bash"}
	dir := "/var/folders/xy/oracle-123"

	for _, line := range []string{
		"exit -1\n",
		"the process group is 78796\n",
		"(-1)\n",
	} {
		got := normalize(line, sh, dir)
		if strings.Contains(got, "(N)") {
			t.Errorf("normalize(%q) = %q, which masked a number nobody named a group", line, got)
		}
	}
}
