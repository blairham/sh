// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Whether `bg` in a shell with no job control refuses before reading its
// operand. Three of the panel say so to anything at all — one of them without
// a word — and dash reads the operand first and complains about that instead.
//
// Refusing is not the axis and is not optional: no shell in the panel resumes
// a job in a shell that has no job control, so the two branches differ in
// what is said and not in whether the job runs (#2657).
func TestBgMayReportAbsentJobControlFirst(t *testing.T) {
	for _, c := range []struct {
		name       string
		first      Answer
		wording    string
		wantStderr string
	}{
		{"refused before the operand", Yes, "%[1]s: no job control", "no job control"},
		// Empty is a member's answer rather than a gap: ksh93 refuses in the
		// same place and prints nothing at all.
		{"refused before the operand, in silence", Yes, "", ""},
		{"the operand is read first", No, "", "--"},
	} {
		t.Run(c.name, func(t *testing.T) {
			sem := PosixSemantics()
			sem.JobControlAbsenceIsReportedFirst = c.first
			errOut := &strings.Builder{}
			r := newTestRunner(t, &Runner{
				Semantics: &sem, Diagnostics: &Diagnostics{NoJobControl: c.wording}, Name: "sh",
				Stdout: &strings.Builder{}, Stderr: errOut,
			})
			f, err := syntax.Parse("bg --version\n", syntax.Core())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatal(err)
			}
			got := errOut.String()
			if c.wantStderr == "" {
				if got != "" {
					t.Errorf("stderr %q, want nothing at all", got)
				}
				return
			}
			if !strings.Contains(got, c.wantStderr) {
				t.Errorf("stderr %q, want it to mention %q", got, c.wantStderr)
			}
		})
	}
}

// A script's `fg` refuses rather than resuming, whichever branch of the axis
// above it takes, and the refusal is a diagnostic: stdout stays empty.
//
// That last part is the reason this is a bug and not a wording difference.
// A script writing `fg 2>/dev/null` — which is what a script writes when it
// does not care whether there was a job — got a line of the job's command
// text on its standard output, because this shell took the request as a live
// one, named the job the way an interactive `fg` does and only then failed
// (#2657). Nothing in the panel does that: ksh93 refuses without a word, dash
// says the job was not created under job control, and bash and zsh refuse
// before they read the operand.
//
// The stops are unannounced with nobody to tell, so whatever these runs
// wrote, `fg` wrote.
func TestAScriptsFgRefusesRatherThanResuming(t *testing.T) {
	for _, c := range []struct {
		name       string
		src        string
		noJob      bool
		shape      func(*Semantics, *Diagnostics)
		wantOut    string
		wantStatus int
	}{
		{
			name: "refused before the operand",
			src:  "fg",
			shape: func(s *Semantics, d *Diagnostics) {
				s.JobControlAbsenceIsReportedFirst = Yes
				d.NoJobControl = "%[1]s: no job control"
			},
			wantOut:    "testsh: fg: no job control\n",
			wantStatus: 1,
		},
		{
			name: "refused before the operand, in silence",
			src:  "fg",
			shape: func(s *Semantics, _ *Diagnostics) {
				s.JobControlAbsenceIsReportedFirst = Yes
			},
			wantOut:    "",
			wantStatus: 1,
		},
		{
			name: "the operand read first, and the refusal names the job it found",
			src:  "fg %1",
			shape: func(s *Semantics, d *Diagnostics) {
				s.JobControlAbsenceIsReportedFirst = No
				d.JobNotUnderJobControl = "%[1]s: job %[2]s not created under job control"
				d.JobNotUnderJobControlStatus = 2
			},
			wantOut:    "testsh: fg: job %1 not created under job control\n",
			wantStatus: 2,
		},
		{
			name: "and names it the way the dialect names an operand there was none of",
			src:  "fg",
			shape: func(s *Semantics, d *Diagnostics) {
				s.JobControlAbsenceIsReportedFirst = No
				d.JobNotUnderJobControl = "%[1]s: job %[2]s not created under job control"
				d.JobNotUnderJobControlStatus = 2
				d.AbsentJobSpec = "(null)"
			},
			wantOut:    "testsh: fg: job (null) not created under job control\n",
			wantStatus: 2,
		},
		{
			name:  "and with nothing in the table, a sentence of its own",
			src:   "fg",
			noJob: true,
			shape: func(s *Semantics, d *Diagnostics) {
				s.JobControlAbsenceIsReportedFirst = No
				d.NoCurrentJob = "%[1]s: No current job"
				d.NoCurrentJobStatus = 2
			},
			wantOut:    "testsh: fg: No current job\n",
			wantStatus: 2,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			// A stopped command is what puts a job in the table; the last
			// case wants none, so it runs the builtin on its own.
			src, waits := echoCmd+"\n"+c.src, []Wait{stopped}
			if c.noJob {
				src, waits = c.src, nil
			}
			out, st, _ := jobSession(t, &fakeJobs{waits: waits}, src, false, c.shape)
			if out != c.wantOut {
				t.Errorf("the run wrote %q, want %q", out, c.wantOut)
			}
			if strings.Contains(out, echoCmd) {
				t.Errorf("the run wrote %q, which names the job — a refusal prints no command", out)
			}
			if st != c.wantStatus {
				t.Errorf("status %d, want %d", st, c.wantStatus)
			}
		})
	}
}
