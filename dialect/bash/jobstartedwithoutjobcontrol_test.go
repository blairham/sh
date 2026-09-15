// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// A job started while the monitor was off cannot be resumed, even once the
// monitor is on.
//
// Such a job runs in the shell's own process group rather than one of its own,
// so there is no group to put in front of the terminal. Measured 2026-09-15 on
// bash 5.3.15, `env -i PATH=/usr/bin:/bin` with a scratch HOME:
//
//	$ bash -c 'sleep 4 & set -m; fg; echo "st=$?"'
//	bash: line 1: fg: job 1 started without job control
//	st=1
//
// bash 3.2.57 says the same. It is the refusal #3020 was filed for: without it
// this shell **resumed** the job and waited it out, so `jobs2.sub` of bash's
// own suite — which bash finishes in 10 ms — took us 30 seconds, the length of
// a `sleep` no reference ever waited for. `make bash-suite` reported it as
// `dialect hung`, which means killed on the sweep's bound rather than never
// terminating.
//
// Pinned here rather than in the corpus on purpose: every spelling of this
// question needs a real background job to resume, so a case costs a `sleep`
// per column on every oracle run — the cost #3021 was filed for — and three of
// the seven columns cannot reach the state at all, since zsh and dash refuse
// `set -m` without a terminal and ash turns the monitor off.
func TestAJobStartedWithoutJobControlIsNotResumedHere(t *testing.T) {
	for _, tc := range []struct{ src, wantOut, wantErr string }{
		{
			`sleep 4 & set -m; fg; echo "st=$?"`,
			"st=1", "fg: job 1 started without job control",
		},
		// `bg` is the same refusal, named for itself.
		{
			`sleep 4 & set -m; bg; echo "st=$?"`,
			"st=1", "bg: job 1 started without job control",
		},
	} {
		out, _ := answersRun(t, tc.src)
		if !strings.Contains(out, tc.wantErr) {
			t.Errorf("%s: said %q, want it to carry %q", tc.src, out, tc.wantErr)
		}
		if !strings.Contains(out, tc.wantOut) {
			t.Errorf("%s: said %q, want it to carry %q", tc.src, out, tc.wantOut)
		}
	}
}

// The two shapes that must still resume, which is what keeps the refusal above
// from being a shell that stopped resuming anything.
//
// A job started *with* the monitor on has a group of its own, and a job with
// no process of its own has no group either way — measured 2026-09-15,
// `set -m; sleep 1 & fg` and `set -m; { sleep 1; } & fg` both resume in bash
// 5.3.15 and in the shipped binary, at status 0.
//
// What is asserted here is narrower than that, and deliberately so: this
// harness runs the interpreter in-process with no job control behind it, so
// neither job has a process to resume and both end at the shell's own
// `this job has no process to resume`. The status therefore says nothing. What
// it can still say is that **the new refusal did not claim them** — which is
// the whole regression risk, since a check written one condition too wide
// would refuse every job rather than only those started without the monitor.
// The resume itself is measured against the real binary, and by
// `make bash-suite`, where the file that drove #3020 now scores instead of
// being killed on the bound.
func TestAJobStartedUnderJobControlIsNotClaimedByTheRefusalHere(t *testing.T) {
	for _, src := range []string{
		`set -m; sleep 1 & fg; echo "st=$?"`,
		`set -m; { sleep 1; } & fg; echo "st=$?"`,
	} {
		out, _ := answersRun(t, src)
		if strings.Contains(out, "started without job control") {
			t.Errorf("%s: refused a job it should not have: %q", src, out)
		}
	}
}
