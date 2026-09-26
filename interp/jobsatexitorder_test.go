// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"
	"syscall"
	"testing"
)

// addRunningJob puts a job in the table that has neither stopped nor
// finished, which is what a `&` job is while it runs.
//
// Built here rather than started, for the reason addStoppedJob is: the
// question is which sentence a *table* draws, and a table is the whole of the
// input. A real job would bring a goroutine and a process with it and answer
// the same question more slowly and less certainly.
func (r *Runner) addRunningJob(pid int, argv []string) {
	job := &Job{
		Command: strings.Join(argv, " "),
		polled:  true,
		done:    make(chan struct{}),
		ready:   make(chan struct{}),
	}
	job.settleStartedPID(pid, true)
	r.addJob(job)
	r.setLastJob(job)
}

// Which of the two jobs-at-exit sentences a table holding one of each draws
// (#4544).
//
// Two readings, and the measurement the rule was first written from could not
// tell them apart because it had the stopped job **first** — the one
// arrangement where both readings agree. See
// Semantics.JobsAtExitSentenceFollowsTheTableOrder for the panel.
//
// The rows are the grid the noun needs: the order is varied while everything
// else is held fixed, so a rule keyed on anything but position cannot pass all
// four. And the `stopped first` rows carry the answer *both* readings give,
// which is what says the axis narrows the question rather than replacing it.
func TestWhichJobsAtExitSentenceATableWithOneOfEachDraws(t *testing.T) {
	for _, tc := range []struct {
		name      string
		follows   Answer
		stopFirst bool
		want      string
	}{
		{"the table decides, running first", Yes, false, "RUNNING"},
		{"the table decides, stopped first", Yes, true, "STOPPED"},
		{"a stopped job wins, running first", No, false, "STOPPED"},
		{"a stopped job wins, stopped first", No, true, "STOPPED"},
		{
			// CONTROL, and the row that says *where* the axis is asked. A
			// stopped job in front of a running one draws the stopped
			// sentence under both readings, so the question is not put and
			// an Unspecified vector must answer rather than refuse. Its
			// mirror — Unspecified with the running job in front — is
			// TestAnUnansweredJobsAtExitOrderSaysNothingElse, which is the
			// proof this instrument can produce a refusal at all.
			"an unanswered axis decides the shape it need not decide",
			Unspecified, true, "STOPPED",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var errs strings.Builder
			sem := CoreSemantics()
			sem.JobsAtExitSentenceFollowsTheTableOrder = tc.follows
			dg := Diagnostics{StoppedJobsAtExit: "STOPPED", RunningJobsAtExit: "RUNNING"}
			r := newTestRunner(t, &Runner{Semantics: &sem, Diagnostics: &dg, Stderr: &errs})
			r.SetChecksRunningJobsAtExit(true)
			if tc.stopFirst {
				r.addStoppedJob(4242, []string{"sleep", "30"}, syscall.SIGTSTP)
				r.addRunningJob(4243, []string{"sleep", "30"})
			} else {
				r.addRunningJob(4243, []string{"sleep", "30"})
				r.addStoppedJob(4242, []string{"sleep", "30"}, syscall.SIGTSTP)
			}
			if got := r.jobsAtExitSentence(); got != tc.want {
				t.Errorf("sentence = %q, want %q", got, tc.want)
			}
			if strings.Contains(errs.String(), "disagree here") {
				t.Errorf("stderr = %q: an answered axis wrote a refusal", errs.String())
			}
		})
	}
}

// An unanswered axis on the shape it does decide writes the refusal and
// nothing else.
//
// Saying one of the two sentences as well would be answering the question it
// has just declined, and the caller reads an empty sentence as "nothing to
// say" — which is the same thing a shell with no jobs at all has.
func TestAnUnansweredJobsAtExitOrderSaysNothingElse(t *testing.T) {
	var errs strings.Builder
	sem := CoreSemantics()
	dg := Diagnostics{StoppedJobsAtExit: "STOPPED", RunningJobsAtExit: "RUNNING"}
	r := newTestRunner(t, &Runner{Semantics: &sem, Diagnostics: &dg, Stderr: &errs})
	r.SetChecksRunningJobsAtExit(true)
	r.addRunningJob(4243, []string{"sleep", "30"})
	r.addStoppedJob(4242, []string{"sleep", "30"}, syscall.SIGTSTP)

	if got := r.jobsAtExitSentence(); got != "" {
		t.Errorf("sentence = %q, want nothing beside the refusal", got)
	}
	if !strings.Contains(errs.String(), "disagree here") {
		t.Errorf("stderr = %q, want the axis named", errs.String())
	}
}

// A job of a kind this shell is not checking is passed over rather than
// counted as the first one.
//
// Measured on the one column that can be asked: with `unsetopt
// checkrunningjobs` at a zsh session, a running job in front of a stopped one
// still draws `you have suspended jobs.` So "the first job" means the first
// job this shell would have said anything about, and a reading that answered
// with silence because the first job's sentence is switched off would be
// wrong.
func TestAJobsAtExitSentenceSkipsAKindThisShellIsNotChecking(t *testing.T) {
	var errs strings.Builder
	sem := CoreSemantics()
	sem.JobsAtExitSentenceFollowsTheTableOrder = Yes
	dg := Diagnostics{StoppedJobsAtExit: "STOPPED", RunningJobsAtExit: "RUNNING"}
	r := newTestRunner(t, &Runner{Semantics: &sem, Diagnostics: &dg, Stderr: &errs})
	// The running half switched off, which is `shopt -u checkjobs` in one
	// dialect and `unsetopt checkrunningjobs` in the other.
	r.SetChecksRunningJobsAtExit(false)
	r.addRunningJob(4243, []string{"sleep", "30"})
	r.addStoppedJob(4242, []string{"sleep", "30"}, syscall.SIGTSTP)

	if got := r.jobsAtExitSentence(); got != "STOPPED" {
		t.Errorf("sentence = %q, want STOPPED: the running job is not this shell's to report", got)
	}
	if strings.Contains(errs.String(), "disagree here") {
		t.Errorf("stderr = %q: the axis was asked where only one sentence is reachable", errs.String())
	}
}
