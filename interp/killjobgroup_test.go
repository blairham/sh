// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// What a `%` job specification points `kill` at (#2291).
//
// dash aims at the job's *process group* and every other column aims at its
// process, which in a script is the difference between `kill %1` working and
// not working at all: a group id is its leader's pid, `set -m` is refused
// without a controlling terminal, and a job that leads no group has no group
// of its number for anything to receive.
//
// The job here is one this shell has no process for, which is the same
// position a real dash is in for the purpose of the question — there is no
// group — and it keeps the case off a fifo and off the clock. What the axis
// decides is whether the spec reaches anything at all, so the status is the
// whole of the assertion.
//
// Both directions and the refusal, because an axis only ever taken cannot be
// told from one that is always taken.
func TestKillJobSpecAimsAtTheGroupAxis(t *testing.T) {
	// The refusal a dialect-less runner prints goes to standard error, so
	// the unanswered row keeps it and the two answered rows drop it — what
	// they assert is the status, and dash's own complaint is the dialect's
	// wording rather than this axis's.
	const src = `( exit 0 ) &
kill -0 %1 2>/dev/null
printf 'st=%s\n' "$?"`
	const loud = `( exit 0 ) &
kill -0 %1
printf 'st=%s\n' "$?"`

	for _, tc := range []struct {
		name    string
		answer  Answer
		want    string
		refused string
	}{
		{
			// bash 5.3.20, zsh 5.9.2, ksh93u+ 2012-08-01, BusyBox ash 1.37.0.
			name: "the spec reaches the job's process", answer: No,
			want: "st=0\n",
		},
		{
			// dash, all three builds. Measured 2026-09-16.
			name: "the spec reaches a group that is not there", answer: Yes,
			want: "st=1\n",
		},
		{
			name: "unanswered", answer: Unspecified,
			refused: "`kill` aiming a job spec at the job's process group",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := getoptsSem()
			sem.KillJobSpecAimsAtTheGroup = tc.answer
			text := src
			if tc.refused != "" {
				text = loud
			}
			out, _ := run(t, text, func(r *Runner) { r.Semantics = &sem })
			if tc.refused != "" {
				if !strings.Contains(out, tc.refused) {
					t.Fatalf("got %q, want it to carry %q", out, tc.refused)
				}
				return
			}
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}
