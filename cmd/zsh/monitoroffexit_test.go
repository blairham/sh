// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// TestAPromptWithTheMonitorOffLeavesItsJobsAlone is #6028: an interactive
// shell whose monitor is off — `-i` on a pipe, where it cannot be turned on —
// leaves at the first `exit` with a running job, says nothing about it, and
// hangs nothing up. Measured 2026-10-05 on zsh 5.9.2 (/opt/homebrew/bin/zsh),
// scratch HOME and ZDOTDIR, the program on a pipe: `STILL` never runs and the
// error stream is silent; the job was still running after the shell left.
func TestAPromptWithTheMonitorOffLeavesItsJobsAlone(t *testing.T) {
	scratchHome(t)
	out, errs, _ := prompt(t, "print M=$options[monitor]\nsleep 1 &\nexit\necho STILL\n", "zsh", "-i")
	if !strings.Contains(out, "M=off") {
		t.Fatalf("the monitor is not off here, so the row asks nothing: stdout %q", out)
	}
	if strings.Contains(out, "STILL") {
		t.Errorf("`exit` held: stdout %q", out)
	}
	for _, said := range []string{"running jobs", "SIGHUPed"} {
		if strings.Contains(errs, said) {
			t.Errorf("stderr %q says %q", errs, said)
		}
	}
}
