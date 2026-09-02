// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"
	"syscall"
	"testing"
)

// A stopped job, which needs a terminal to make and so is made here directly.
// Two rules meet on its line and neither is the format string.
func TestTheLineOfAStoppedJob(t *testing.T) {
	for _, tc := range []struct {
		name string
		dg   Diagnostics
		show Answer
		want string
	}{
		{
			// Every shell in the panel prints a stopped job's command,
			// including both of the two that print nothing for a `&` job —
			// which is the whole reason that axis is about `&` jobs and not
			// about commands.
			"its command is never the hidden one",
			Diagnostics{JobUnknownCommand: "<command unknown>"},
			No,
			"sleep 30",
		},
		{
			// One dialect names the signal instead of calling it stopped, so
			// the number has to travel on the job to reach the wording.
			"and its state can name the signal",
			Diagnostics{JobStopped: "Suspended: %[1]d"},
			Yes,
			"Suspended: 18",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := CoreSemantics()
			sem.JobsShowBackgroundCommand = tc.show
			r := &Runner{Semantics: &sem, Diagnostics: &tc.dg}
			r.addStoppedJob(1, []string{"sleep", "30"}, syscall.SIGTSTP)

			line := r.jobLine(0, r.jobs[0], tc.show == Yes)
			if !strings.Contains(line, tc.want) {
				t.Errorf("line = %q, want it to contain %q", line, tc.want)
			}
		})
	}
}
