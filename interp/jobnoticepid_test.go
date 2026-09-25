// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// Semantics.JobNoticeNamesThePID decides whether a job *notice* carries the
// job's process id — the long row a `-l` listing writes, rather than the short
// one a bare listing writes.
//
// Every case below is run in **both** states, because the half that was
// already right is the half that must not be traded away: a shell that names
// the pid unconditionally passes a row that only ever asked it one question.
// The axis left unanswered is run too, and it goes with the short form — the
// whole panel's reading, and not a place to refuse a script mid-notice.
//
// The wordings here are the defaults rather than any dialect's. This package
// names an axis and never a shell.
func TestANoticeNamesThePIDWhereTheAxisSaysSo(t *testing.T) {
	const pid = 4711
	for _, surface := range []struct {
		name string
		// line renders one surface of a shell holding one stopped job.
		line func(r *Runner, j *Job) string
		// short and long are what that surface writes in the two states.
		short, long string
	}{
		{
			// The notice a stopped job gets, which is the shape the
			// asynchronous ones share.
			name:  "a notice",
			line:  func(r *Runner, j *Job) string { return r.jobNoticeLine(1, j, true, false) },
			short: "[1]+  Stopped                 sleep 30",
			long:  "[1]+ 4711 Stopped                 sleep 30",
		},
		{
			// `fg` and `bg` name the job they resumed, and that is a notice
			// too: the state word is the resume's own and everything around
			// it is the listing's row.
			name: "a resume notice",
			line: func(r *Runner, j *Job) string {
				return r.resumeNotice(j, "", "sleep 30", "Continued")
			},
			short: "sleep 30",
			long:  "[1]+ 4711 Continued               sleep 30",
		},
		{
			// And the listing is a different surface, which is the pair that
			// says what the axis is keyed on: the same job, in the same
			// state, in the same shell, is listed short while it is noticed
			// long.
			name:  "a listing, which the axis does not reach",
			line:  func(r *Runner, j *Job) string { return r.jobLine(1, j, true) },
			short: "[1]+  Stopped                 sleep 30",
			long:  "[1]+  Stopped                 sleep 30",
		},
	} {
		t.Run(surface.name, func(t *testing.T) {
			for _, state := range []struct {
				name   string
				answer Answer
				want   string
			}{
				{"named", Yes, surface.long},
				{"not named", No, surface.short},
				{"and an unanswered axis takes the panel's reading", Unspecified, surface.short},
			} {
				t.Run(state.name, func(t *testing.T) {
					sem := CoreSemantics()
					sem.JobsShowBackgroundCommand = Yes
					sem.JobNoticeNamesThePID = state.answer
					dg := Diagnostics{}
					r := newTestRunner(t, &Runner{Semantics: &sem, Diagnostics: &dg})
					r.addStoppedJob(pid, []string{"sleep", "30"}, syscall.SIGTSTP)
					if got := surface.line(r, r.jobs[0]); got != state.want {
						t.Errorf("line = %q, want %q", got, state.want)
					}
				})
			}
		})
	}
}

// The state word travels into the long row rather than being fixed there,
// which is the one thing the resume notice adds over the others: a job that
// was running when it was named keeps `Running`, and only one the shell had to
// continue is called continued.
//
// Asserted apart from the grid above because it is the mutant that grid cannot
// kill: a long row that ignored the state argument and rendered the job's own
// would still write the right thing for the stopped case.
func TestAResumeNoticeCarriesItsOwnStateWordIntoTheLongRow(t *testing.T) {
	sem := CoreSemantics()
	sem.JobsShowBackgroundCommand = Yes
	sem.JobNoticeNamesThePID = Yes
	dg := Diagnostics{}
	r := newTestRunner(t, &Runner{Semantics: &sem, Diagnostics: &dg})
	r.addStoppedJob(4711, []string{"sleep", "30"}, syscall.SIGTSTP)
	got := r.resumeNotice(r.jobs[0], "", "sleep 30", "Running")
	const want = "[1]+ 4711 Running                 sleep 30"
	if got != want {
		t.Errorf("notice = %q, want %q — the resume's word, not the job's", got, want)
	}
}

// And the signal the state word can name still reaches it through the long
// row, which is the second argument the row has to pass along.
//
// The number is not written out: SIGTSTP is 18 on a BSD and 20 on Linux, so
// hardcoding either tests the platform rather than the rendering.
func TestTheLongNoticeStillNamesTheStoppingSignal(t *testing.T) {
	sem := CoreSemantics()
	sem.JobsShowBackgroundCommand = Yes
	sem.JobNoticeNamesThePID = Yes
	dg := Diagnostics{JobStopped: "Suspended: %[1]d"}
	r := newTestRunner(t, &Runner{Semantics: &sem, Diagnostics: &dg})
	r.addStoppedJob(4711, []string{"sleep", "30"}, syscall.SIGTSTP)
	line := r.jobNoticeLine(1, r.jobs[0], true, false)
	want := "Suspended: " + strconv.Itoa(int(syscall.SIGTSTP))
	if !strings.Contains(line, want) {
		t.Errorf("line = %q, want it to contain %q", line, want)
	}
	if !strings.Contains(line, "4711") {
		t.Errorf("line = %q, want the pid in it too", line)
	}
}
