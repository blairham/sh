// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A `jobs` listing shows what was typed, and by the time it is asked the words
// have been expanded and the process started — so the text has to be kept when
// the command is read, or it is gone.
//
// It was not kept at all: the column was there and always empty, which reads
// as broken rather than as missing. A *stopped* job printed its text, because
// that path had the argv, so the two halves of one listing disagreed.
func TestABackgroundJobRemembersWhatWasTyped(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a command", `true &`, "true"},
		// Taken from the input rather than rebuilt from the tree, so the
		// spelling survives: a printer would show what the parser understood.
		{"spacing and quoting survive", `true "a  b" &`, `true "a  b"`},
		{"a pipeline", `true | cat &`, "true | cat"},
		// `&` binds to the statement, so the whole and-or is the job.
		{"an and-or", `true && echo x &`, "true && echo x"},
		{"a compound command", `{ true; } &`, "{ true; }"},
		{"no space before the ampersand", `true&`, "true"},
		// Recorded by the parser, so it does not matter who runs it or how
		// long after: a runner holding the text of the file it is running
		// would have the wrong file here.
		{"inside a function", `f(){ true & }; f`, "true"},
		{"inside an eval", `eval "true &"`, "true"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var r *Runner
			if _, st := run(t, tc.src, func(rr *Runner) { r = rr }); st != 0 {
				t.Fatalf("status %d", st)
			}
			jobs := r.Jobs()
			if len(jobs) != 1 {
				t.Fatalf("%d jobs, want the one that was started", len(jobs))
			}
			if got := jobs[0].Command; got != tc.want {
				t.Errorf("Command = %q, want %q", got, tc.want)
			}
		})
	}
}

// A backgrounded pipeline is one job with one pid, and it is the *last*
// element's — bash and zsh agree, and `sleep 1 | cat &` makes `$!` the `cat`.
//
// Every element used to carry the job, so every element wrote the same
// Job.PID from its own goroutine: a data race the detector catches on the
// test above, and wrong for all but one of them. Clearing the job from all of
// them is the other way to get this wrong, and this is what says so.
func TestABackgroundedPipelineTakesTheLastElementsPid(t *testing.T) {
	var r *Runner
	// Externals, because a builtin has no process and so no pid to take.
	if _, st := run(t, `/usr/bin/true | /usr/bin/true &`, func(rr *Runner) {
		sem := CoreSemantics()
		// The dialect where the last element is a child rather than this
		// shell, which is the arrangement that had every element writing.
		sem.LastPipelineElementInCurrentShell = No
		rr.Semantics = &sem
		r = rr
	}); st != 0 {
		t.Fatalf("status %d", st)
	}
	jobs := r.Jobs()
	if len(jobs) != 1 {
		t.Fatalf("%d jobs, want one", len(jobs))
	}
	if jobs[0].PID == 0 {
		t.Error("PID = 0 — the job took no element's process at all")
	}
	// And `$!` names the same process, which is the observable half.
	out, _ := run(t, `/usr/bin/true | /usr/bin/true & echo "$!"`, func(rr *Runner) {
		sem := CoreSemantics()
		sem.LastPipelineElementInCurrentShell = No
		rr.Semantics = &sem
	})
	if strings.TrimSpace(out) == "0" || strings.TrimSpace(out) == "" {
		t.Errorf("$! = %q, want the last element's process", out)
	}
}

// Only a background statement pays for its text. Nothing else needs it: every
// other node can be re-read from the input it came from, and a `jobs` listing
// is the one place a command has to be shown back long after it was read.
func TestOnlyABackgroundStatementKeepsItsText(t *testing.T) {
	var r *Runner
	if _, st := run(t, `true & echo done`, func(rr *Runner) { r = rr }); st != 0 {
		t.Fatalf("status %d", st)
	}
	if got := r.Jobs()[0].Command; got != "true" {
		t.Errorf("Command = %q, want only the backgrounded half", got)
	}
}
