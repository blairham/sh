// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// After a held `exit`, the next `exit` leaves only when it is the very next
// named command the shell runs (#6080). See
// interp.Semantics.HeldExitWaivedForTheNextCommandOnly.
//
// Measured 2026-10-05 on bash 5.3.20, `--norc -i` with a scratch HOME,
// `shopt -s checkjobs` and a job running, through a pseudo-terminal and on a
// pipe alike: `exit`, then each line below, then `echo STILL-$((40+2))`. A
// named command between the two holds the second `exit`, and an assignment, a
// `[[ ]]` or a subshell does not. `exit` twice is the control.
func TestAHeldExitIsWaivedForTheNextCommandOnly(t *testing.T) {
	for _, c := range []struct {
		lines string
		stays bool
	}{
		{"true; exit\n", true},
		{"eval exit\n", true},
		{"f(){ exit; }; f\n", true},
		{":\nexit\n", true},
		{"x=1\nexit\n", false},
		{"[[ 1 ]]\nexit\n", false},
		{"(exit)\nexit\n", false},
		{"exit\n", false},
	} {
		t.Run(strings.TrimSpace(c.lines), func(t *testing.T) {
			scratchHome(t)
			out, _, _ := prompt(t, "shopt -s checkjobs\nsleep 2 &\nexit\n"+c.lines+"echo STILL-$((40+2))\nkill %1; wait\n", "bash", "--norc", "-i")
			if got := strings.Contains(out, "STILL-42"); got != c.stays {
				t.Errorf("stayed = %v, want %v; stdout %q", got, c.stays, out)
			}
		})
	}
}
