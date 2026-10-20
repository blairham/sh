// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// A job that has already ended is refused by `fg` and `bg`, before anything is
// printed for it.
//
// Measured 2026-10-04 on bash 5.3 (/opt/homebrew/bin/bash) and 3.2.57, from a
// file with no terminal, `bash fg.sh </dev/null`:
//
//	set -m
//	sleep 0 &
//	sleep 0.5
//	fg
//	echo "rc=$?"
//
//	fg.sh: line 4: fg: job has terminated
//	rc=1
//
// and `bg` the same, named for itself. This shell printed the job's command
// line on stdout — the notice of a job being put back in front — and only then
// failed on the continue, with `no such process`; `bg` called the ended job
// `already in background` and reported 0 (#5791).
//
// A job with no process of its own is used here because this harness runs the
// interpreter in-process, where no job has one. That shape is refused the same
// way in bash: `set -m; { :; } & sleep 0.3; fg` says `fg: job has terminated`
// at 1. What separates the fix from the old path is the stdout line and the
// wording — the old path reached `this job has no process to resume` after
// printing the notice.
func TestAnEndedJobIsRefusedByFgAndBgHere(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{
			`set -m; { :; } & sleep 0.5; fg; echo "st=$?"`,
			"fg: job has terminated",
		},
		{
			`set -m; { :; } & sleep 0.5; bg; echo "st=$?"`,
			"bg: job has terminated",
		},
	} {
		out, _ := answersRun(t, tc.src)
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s: said %q, want it to carry %q", tc.src, out, tc.want)
		}
		if !strings.Contains(out, "st=1") {
			t.Errorf("%s: said %q, want st=1", tc.src, out)
		}
		for _, line := range strings.Split(out, "\n") {
			if strings.Contains(line, "has terminated") || strings.HasPrefix(line, "st=") || line == "" {
				continue
			}
			t.Errorf("%s: said %q beside the refusal, want nothing else", tc.src, line)
		}
	}
}
