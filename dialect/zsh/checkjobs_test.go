// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"context"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// This shell's two names for looking at the job table before it leaves, and
// the relationship between them.
//
// Measured 2026-09-08 through a pseudo-terminal against zsh 5.9.2, with the
// job started and then `exit` typed:
//
//   - both on, which is where this shell starts: a `sleep 40 &` is
//     `you have running jobs.` and a job suspended with ^Z is
//     `you have suspended jobs.`;
//   - `unsetopt checkrunningjobs`: the running one leaves at once and the
//     suspended one still holds;
//   - `unsetopt checkjobs`: both leave at once, which is where this shell
//     differs from bash — `shopt -u checkjobs` there still holds for a
//     suspended job.
func TestCheckJobsIsTheMasterAndCheckRunningJobsNarrowsIt(t *testing.T) {
	for _, tc := range []struct {
		src              string
		stopped, running bool
	}{
		{``, true, true},
		{`unsetopt checkrunningjobs`, true, false},
		{`unsetopt checkjobs`, false, false},
		// The master turned off takes the running half with it, and turning
		// it back on restores what the narrower name was left saying — which
		// is the whole reason that name keeps a bit of its own rather than
		// reading the core switch the master zeroes.
		{`unsetopt checkjobs; setopt checkjobs`, true, true},
		{`unsetopt checkrunningjobs; unsetopt checkjobs; setopt checkjobs`, true, false},
		{`unsetopt checkjobs; setopt checkrunningjobs`, false, false},
	} {
		t.Run(tc.src, func(t *testing.T) {
			r := preset.Runner(dialecttest.Base{Dir: t.TempDir()})
			if tc.src != "" {
				f := preset.Parse(t, tc.src)
				if _, err := r.Run(context.Background(), f); err != nil {
					t.Fatalf("%s: %v", tc.src, err)
				}
			}
			if got := r.ChecksStoppedJobsAtExit(); got != tc.stopped {
				t.Errorf("ChecksStoppedJobsAtExit() = %v, want %v", got, tc.stopped)
			}
			if got := r.ChecksRunningJobsAtExit(); got != tc.running {
				t.Errorf("ChecksRunningJobsAtExit() = %v, want %v", got, tc.running)
			}
		})
	}
}

// And both names read back through the option namespace, in both directions —
// a switch that moved the core state without the listing following it would
// leave `setopt` describing a shell that no longer exists.
func TestBothJobCheckNamesReadBack(t *testing.T) {
	for _, tc := range []struct {
		src, name string
		want      bool
	}{
		{``, "checkjobs", true},
		{``, "checkrunningjobs", true},
		{`unsetopt checkjobs`, "checkjobs", false},
		{`unsetopt checkrunningjobs`, "checkrunningjobs", false},
		// The narrower name is remembered across the master moving, which is
		// the state a core switch alone cannot hold.
		{`unsetopt checkrunningjobs; unsetopt checkjobs`, "checkrunningjobs", false},
		{`unsetopt checkjobs; setopt checkjobs`, "checkrunningjobs", true},
	} {
		t.Run(tc.src+" "+tc.name, func(t *testing.T) {
			r := preset.Runner(dialecttest.Base{Dir: t.TempDir()})
			if tc.src != "" {
				f := preset.Parse(t, tc.src)
				if _, err := r.Run(context.Background(), f); err != nil {
					t.Fatalf("%s: %v", tc.src, err)
				}
			}
			got, known := r.DialectOption(tc.name)
			if !known {
				t.Fatalf("%s is not a name in this shell's namespace", tc.name)
			}
			if got != tc.want {
				t.Errorf("%s = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}
