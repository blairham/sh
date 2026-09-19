// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Whether `kill` refuses the all-processes target (#2145).
//
// ksh93u+ alone refuses the pid `-1`, and refuses it by name rather than as a
// process group it could not reach. Measured 2026-09-19 with signal 0, which
// runs the kernel's permission check and delivers nothing — which is what
// made the panel runnable on a working machine instead of argued about:
//
//	kill -0 -1        ksh93: `kill: -1: permission denied`, 1
//	                     bash 5.3.20, zsh 5.9.2, dash 0.5.12, ash 1.37.0: 0
//
// Every row here uses signal 0 for the same reason, so the test asks the
// question without ever sending anything.
//
// The two controls are the point. An axis that refused every negative pid, or
// every pid, would pass the first two rows and be wrong — so one row aims at
// a process group that is absent and requires the *other* complaint, and one
// aims at this process and requires no complaint at all.
func TestKillRefusesTheAllProcessesTargetAxis(t *testing.T) {
	for _, tc := range []struct {
		name     string
		answer   Answer
		src      string
		want     string
		contains string
		refused  string
	}{
		{
			// bash, zsh, dash, ash: the operand goes to the kernel.
			name: "the target is sent", answer: No,
			src:  "kill -0 -1 2>/dev/null; printf 'st=%s\\n' \"$?\"",
			want: "st=0\n",
		},
		{
			// ksh93u+.
			name: "the target is refused", answer: Yes,
			src:  "kill -0 -1 2>/dev/null; printf 'st=%s\\n' \"$?\"",
			want: "st=1\n",
		},
		{
			// CONTROL. A process group that is not there is still ESRCH
			// under the refusing answer — the axis names one pid, not the
			// sign. Broaden it to every negative number and this row starts
			// reporting the refusal's wording instead.
			name: "an absent group keeps its own complaint", answer: Yes,
			src:      "kill -0 -99999; printf 'st=%s\\n' \"$?\"",
			contains: "no such process",
		},
		{
			// CONTROL. This process, under the refusing answer, is reached.
			name: "this process is still reachable", answer: Yes,
			src:  "kill -0 \"$$\" 2>/dev/null; printf 'st=%s\\n' \"$?\"",
			want: "st=0\n",
		},
		{
			name: "unanswered", answer: Unspecified,
			src:     "kill -0 -1; printf 'st=%s\\n' \"$?\"",
			refused: "`kill` refusing the all-processes target",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := getoptsSem()
			sem.KillRefusesTheAllProcessesTarget = tc.answer
			out, _ := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			switch {
			case tc.refused != "":
				if !strings.Contains(out, tc.refused) {
					t.Fatalf("got %q, want it to carry %q", out, tc.refused)
				}
			case tc.contains != "":
				if !strings.Contains(strings.ToLower(out), tc.contains) {
					t.Fatalf("got %q, want it to carry %q", out, tc.contains)
				}
				if strings.Contains(strings.ToLower(out), "permission denied") {
					t.Fatalf("got %q, which is the all-processes refusal on a target that is not it", out)
				}
			default:
				if out != tc.want {
					t.Errorf("got %q, want %q", out, tc.want)
				}
			}
		})
	}
}
