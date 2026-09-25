// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// What each dialect says about a job it is asked to background or resume, in
// the three places the panel disagrees and the columns are not the same
// columns twice.
//
// One table rather than a row in each dialect's own file, because the point is
// that the three splits cut differently: zsh alone announces a job on the
// monitor, zsh alone words its resume notice as a listing row, and bash and
// zsh refuse `bg` for a running job with different statuses while the other
// three resume it. A per-dialect assertion states each value and none of them
// states that (#2838).
//
// Measured 2026-09-15, `env -i PATH=/usr/bin:/bin HOME=<scratch>`, a script
// file with `set -m` on a pseudo-terminal, and again at an interactive prompt.
func TestWhatEachDialectSaysAboutAResumedJob(t *testing.T) {
	for _, tc := range []struct {
		name string
		sem  interp.Semantics
		diag interp.Diagnostics
		// announces is Semantics.MonitorAloneAnnouncesAJob.
		announces interp.Answer
		// alreadyBg and its status are the `bg` refusal; empty is a dialect
		// that resumes the job and prints the usual notice.
		alreadyBg       string
		alreadyBgStatus int
		// resumedFg is the wording `fg` uses, and the one that carries the
		// state verb is the one dialect whose notice is a listing row. Its
		// state column is spelled as that dialect's JobLine spells it —
		// nine wide with two spaces after it rather than a flat eleven —
		// because it *is* that column. The two spellings are byte-identical
		// for every word this row can hold, `running`, `suspended` and
		// `continued` all being nine or fewer; what parts them is a word
		// longer than nine, which only the signal names reach (#4508).
		resumedFg string
	}{
		{
			name: "bash", sem: bash.Semantics(), diag: bash.Diagnostics(),
			announces:       interp.No,
			alreadyBg:       "%[1]s: job %[2]d already in background",
			alreadyBgStatus: 0,
		},
		{
			name: "zsh", sem: zsh.Semantics(), diag: zsh.Diagnostics(),
			announces:       interp.Yes,
			alreadyBg:       "job already in background",
			alreadyBgStatus: 1,
			resumedFg:       "[%[1]d]  %[2]s %-9[4]s  %[3]s",
		},
		{name: "ksh", sem: ksh.Semantics(), diag: ksh.Diagnostics(), announces: interp.No},
		{name: "dash", sem: dash.Semantics(), diag: dash.Diagnostics(), announces: interp.No},
		{name: "ash", sem: ash.Semantics(), diag: ash.Diagnostics(), announces: interp.No},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.sem.MonitorAloneAnnouncesAJob; got != tc.announces {
				t.Errorf("MonitorAloneAnnouncesAJob = %v, want %v", got, tc.announces)
			}
			if got := tc.diag.JobAlreadyInBackground; got != tc.alreadyBg {
				t.Errorf("JobAlreadyInBackground = %q, want %q", got, tc.alreadyBg)
			}
			if got := tc.diag.JobAlreadyInBackgroundStatus; got != tc.alreadyBgStatus {
				t.Errorf("JobAlreadyInBackgroundStatus = %d, want %d", got, tc.alreadyBgStatus)
			}
			if tc.resumedFg != "" && tc.diag.JobResumedInForeground != tc.resumedFg {
				t.Errorf("JobResumedInForeground = %q, want %q", tc.diag.JobResumedInForeground, tc.resumedFg)
			}
		})
	}
	// And the word itself lives where a state word belongs, in the one
	// dialect that prints one: `continued` is the state column of a listing
	// row, not a sentence, so `fg` on a job that was already running prints
	// `running` there instead.
	if got, want := zsh.Diagnostics().JobContinued, "continued"; got != want {
		t.Errorf("zsh JobContinued = %q, want %q", got, want)
	}
	for _, d := range []struct {
		name string
		diag interp.Diagnostics
	}{
		{"bash", bash.Diagnostics()},
		{"ksh", ksh.Diagnostics()},
		{"dash", dash.Diagnostics()},
		{"ash", ash.Diagnostics()},
	} {
		if got := d.diag.JobContinued; got != "" {
			t.Errorf("%s JobContinued = %q, want nothing: its notice carries no state", d.name, got)
		}
	}
}
