// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// killJobDiag words the three complaints these cases tell apart, one line
// each.
func killJobDiag() Diagnostics {
	d := killManyDiag()
	d.KillNoSuchJob = "kill: %[1]s: no such job"
	return d
}

// Whether the operands behind a `%` spec that names no job are still operands
// (#4666).
//
// A neighbor of KillKeepsGoingPastAnOperandThatIsNotAPid and a separate
// question, which is the whole reason it is a second axis: the two have
// different column patterns, so a shell cannot answer both from one value.
// The pair of rows that says so is here — the malformed-word axis answered
// **Yes** throughout, while this one moves — because it is exactly the
// arrangement BusyBox ash holds and the one a widened first axis could not
// express.
func TestKillKeepsGoingPastAJobSpecThatNamesNoJobAxis(t *testing.T) {
	for _, tc := range []struct {
		name    string
		answer  Answer
		src     string
		want    []string
		refused string
	}{
		{
			// bash 5.3.20 and zsh 5.9.2.
			name: "every operand behind the missing job is reported", answer: Yes,
			src:  "kill %99 a",
			want: []string{"%99: no such job", "illegal pid: a"},
		},
		{
			// dash 0.5.12 and BusyBox ash: the missing job ends it.
			name: "the missing job ends it", answer: No,
			src:  "kill %99 a",
			want: []string{"%99: no such job"},
		},
		{
			// THE PAIR THAT MAKES IT A SECOND AXIS. Both rows above and both
			// rows here run with the malformed-word axis answered Yes, so a
			// shell reading one value for both questions could not produce
			// the `No` row above beside the `Yes` row here — which is what
			// BusyBox ash is.
			name: "and a malformed word behind it is still its own question", answer: No,
			src:  "kill %99 a b",
			want: []string{"%99: no such job"},
		},
		{
			name: "which carries on when it is asked", answer: Yes,
			src:  "kill %99 a b",
			want: []string{"%99: no such job", "illegal pid: a", "illegal pid: b"},
		},
		{
			// CONTROL. Nothing stands behind the spec, so the question is
			// never put — Unspecified rather than an answer, because that is
			// what makes this a test of *where* the question is asked.
			name: "a single operand does not ask the question", answer: Unspecified,
			src:  "kill %99",
			want: []string{"%99: no such job"},
		},
		{
			name: "unanswered", answer: Unspecified,
			src:     "kill %99 a",
			refused: "operands behind a job spec that names no job",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := killSem()
			sem.KillKeepsGoingPastAnOperandThatIsNotAPid = Yes
			sem.KillReadsJobSpecsBeforeTheOtherOperands = No
			sem.KillKeepsGoingPastAJobSpecThatNamesNoJob = tc.answer
			_, errs, st := killRun(t, tc.src, sem, killJobDiag())
			if tc.refused != "" {
				if !strings.Contains(errs, tc.refused) {
					t.Fatalf("stderr = %q, want a refusal naming %q", errs, tc.refused)
				}
				if st != 2 {
					t.Errorf("status = %d, want 2 for an unanswered axis", st)
				}
				if strings.Contains(errs, "no such job") {
					t.Errorf("stderr = %q: the refusal was followed by an answer", errs)
				}
				return
			}
			assertKillLines(t, errs, tc.want)
		})
	}
}

// And whether the `%` operands are read before the others at all (#4666).
//
// One column resolves every job spec first, so a spec that names no job ends
// the builtin with the operands in front of it neither reported nor delivered
// to. The delivery row is the one that says this is an ordering rule about the
// whole builtin rather than about which diagnostic comes first.
func TestKillReadsJobSpecsBeforeTheOtherOperandsAxis(t *testing.T) {
	for _, tc := range []struct {
		name    string
		answer  Answer
		src     string
		want    []string
		refused string
	}{
		{
			// BusyBox ash: `a` is never looked at.
			name: "the job spec is read first", answer: Yes,
			src:  "kill a %99",
			want: []string{"%99: no such job"},
		},
		{
			// bash 5.3.20 and zsh 5.9.2, which carry on: left to right.
			name: "or in the order they were written", answer: No,
			src:  "kill a %99",
			want: []string{"illegal pid: a", "%99: no such job"},
		},
		{
			// The **first** missing spec is the one such a shell reports,
			// and this is why the search does not start behind position
			// zero: `%98` is the second missing one, and a reading that
			// skipped ahead would have named it.
			name: "the first missing spec is the one reported", answer: Yes,
			src:  "kill a %99 %98",
			want: []string{"%99: no such job"},
		},
		{
			// CONTROL. A spec that names no job in *front* of everything
			// orders nothing — every column reports it first either way — so
			// the question is not put, and an Unspecified vector must not
			// refuse here. Two lines, because the axis above is answered Yes
			// and this one was never reached.
			name: "a missing spec in front asks nothing", answer: Unspecified,
			src:  "kill %99 a",
			want: []string{"%99: no such job", "illegal pid: a"},
		},
		{
			// CONTROL, and the input half of the rule: a `%` operand that
			// *does* name a job reaches no question at all, so an ordinary
			// `kill %1 a` runs under an unanswered vector. The job is a
			// background builtin, so the case starts no process and waits
			// for nothing — `&` settles the job before the next command.
			name: "a spec that names a job asks nothing", answer: Unspecified,
			src:  ": & kill %1 a",
			want: []string{"illegal pid: a"},
		},
		{
			name: "unanswered", answer: Unspecified,
			src:     "kill a %99",
			refused: "a job spec that names no job behind another operand",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := killSem()
			sem.KillKeepsGoingPastAnOperandThatIsNotAPid = Yes
			sem.KillKeepsGoingPastAJobSpecThatNamesNoJob = Yes
			sem.KillReadsJobSpecsBeforeTheOtherOperands = tc.answer
			_, errs, st := killRun(t, tc.src, sem, killJobDiag())
			if tc.refused != "" {
				if !strings.Contains(errs, tc.refused) {
					t.Fatalf("stderr = %q, want a refusal naming %q", errs, tc.refused)
				}
				if st != 2 {
					t.Errorf("status = %d, want 2 for an unanswered axis", st)
				}
				return
			}
			assertKillLines(t, errs, tc.want)
		})
	}
}

// Nothing is delivered before the specs have been read, in the column that
// reads them first.
//
// The row that makes the axis an ordering rule about the **builtin** rather
// than about which diagnostic comes first. Measured against BusyBox v1.37.0:
// `sleep 30 & kill $! %99` writes the job's complaint alone, and a `jobs`
// after it still lists that job as running — so the live process in front of
// the spec received nothing.
//
// The discriminator here is the **status** and not the output, and that is the
// point: both answers write the same one line, so a test reading stderr could
// not tell them apart. `kill -0 $$` reaches a process that is certainly there,
// so under `KillStatusAnySuccess` a run that delivered anything reports 0 and
// a run that delivered nothing reports the argument status. Signal 0 rather
// than a real one, so the test binary survives being the target.
func TestKillDeliversNothingWhenAJobSpecIsReadFirst(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer Answer
		status int
	}{
		{"read first, so nothing is signaled", Yes, 1},
		{"read in order, so the process is signaled", No, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := killSem()
			sem.KillStatus = KillStatusAnySuccess
			sem.KillKeepsGoingPastAnOperandThatIsNotAPid = Yes
			sem.KillKeepsGoingPastAJobSpecThatNamesNoJob = Yes
			sem.KillReadsJobSpecsBeforeTheOtherOperands = tc.answer
			_, errs, st := killRun(t, `kill -0 $$ %99`, sem, killJobDiag())
			assertKillLines(t, errs, []string{"%99: no such job"})
			if st != tc.status {
				t.Errorf("status = %d, want %d (stderr %q)", st, tc.status, errs)
			}
		})
	}
}

func assertKillLines(t *testing.T, errs string, want []string) {
	t.Helper()
	got := killManyErrLines(errs)
	if len(got) != len(want) {
		t.Fatalf("stderr = %q: %d lines, want %d", errs, len(got), len(want))
	}
	for i, w := range want {
		if !strings.Contains(got[i], w) {
			t.Errorf("line %d = %q, want it to carry %q", i, got[i], w)
		}
	}
}
