// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A notice waits for a prompt that is not a continuation. The line being typed
// is unfinished, and putting a notice in the middle of it says the same thing
// about the display that one arriving mid-line does.
//
// Asserted here rather than through a session, because reproducing it that way
// needs a job to finish in the seconds between two lines of a construct being
// *typed* — and lines arriving down a pipe leave no such seconds.
func TestANoticeWaitsForAPromptThatIsNotAContinuation(t *testing.T) {
	for _, tc := range []struct {
		name       string
		pending    string
		wantNotice bool
		wantPrompt string
	}{
		{"a fresh line takes both", "", true, "$ "},
		{"a continuation takes neither", "for i in 1; do", false, "> "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var errs strings.Builder
			s := Shell{Runner: runnerWithAFinishedJob(t), Out: &strings.Builder{}, Err: &errs}
			var pending strings.Builder
			pending.WriteString(tc.pending)

			if got := s.beforeReading(&pending); got != tc.wantPrompt {
				t.Errorf("prompt = %q, want %q", got, tc.wantPrompt)
			}
			if got := strings.Contains(errs.String(), "Done"); got != tc.wantNotice {
				t.Errorf("reported = %v, want %v (err %q)", got, tc.wantNotice, errs.String())
			}
		})
	}
}

// runnerWithAFinishedJob is a shell with one job that has already ended.
func runnerWithAFinishedJob(t *testing.T) *interp.Runner {
	t.Helper()
	sem := interp.PosixSemantics()
	sem.AnnouncesBackgroundJob = interp.No
	sem.JobsShowBackgroundCommand = interp.Yes
	r := &interp.Runner{Semantics: &sem, JobControl: true, Stdout: &strings.Builder{}, Stderr: &strings.Builder{}}
	f, perr := syntax.Parse("true &", syntax.Core())
	if perr != nil {
		t.Fatal(perr)
	}
	if err := r.RunPart(t.Context(), f); err != nil {
		t.Fatal(err)
	}
	// Waited for rather than assumed. The comment here used to say the job
	// was a builtin with no process of its own and so had finished by the
	// time backgrounding it returned — which is true almost always, and this
	// test failed about once in three hundred runs because "almost" is not
	// "always". A job is finished when it says it is.
	for _, j := range r.Jobs() {
		j.Wait()
	}
	return r
}
