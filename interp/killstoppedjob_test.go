// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"bytes"
	"context"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// A signal aimed at a **stopped** job, and whether the shell continues it
// first so that the signal can land.
//
// zsh does; nobody else does. Measured 2026-09-25 through a pseudo-terminal —
// `TERM=dumb`, `PS1`/`PS2` exported empty, each shell started interactive —
// with `sleep 300 &` then `kill -STOP %1`, and the process's state read from
// **outside** the shell with `ps -o stat=` on the pid rather than out of the
// job table, which is the shell's own claim about what it just did:
//
//	                     kill -0 %1   kill -WINCH %1   kill -CONT %1
//	zsh 5.9.2            SN           SN               SN
//	bash 5.3.20          T            T                S
//	bash 3.2.57          T            T                S
//	ksh93u+              T            T                S
//	dash 0.5.12          T            T                S
//	BusyBox ash 1.37.0   T            T                S
//
// The rows that discriminate are the ones that deliver nothing anybody can
// act on. A Darwin kernel ends a stopped process on a default-fatal signal
// without waiting to be continued, so `kill -TERM %1` is `GONE` in every
// column on this machine and grades nothing; on Linux the same row reads `T`
// in every column but zsh, which is where the missing continue is a silent
// no-op rather than a mis-worded notice. `kill -CONT %1` is the control, and
// every column moves on it — so the two columns to its left are a difference
// between the shells and not a probe that could not see a continue.
//
// Here the group signal is recorded rather than sent, for the reason the rest
// of the job tests record it: what this file is about is the decision, and a
// test that asked the kernel would be grading the kernel. The `ps` rows above
// are what say the decision is the right one.

// killStoppedShell is a shell holding one stopped job with a process group of
// its own, and the log of every group signal it is asked to send.
func killStoppedShell(t *testing.T, answer Answer) (*Runner, *[]syscall.Signal, *bytes.Buffer) {
	t.Helper()
	// A pid nothing in this process tree has, because every send that
	// reaches it here is the fake below. The one test that needs the kernel
	// to answer names its own.
	return killStoppedShellPID(t, answer, 4242)
}

func killStoppedShellPID(t *testing.T, answer Answer, pid int) (*Runner, *[]syscall.Signal, *bytes.Buffer) {
	t.Helper()
	sem := CoreSemantics()
	sem.KillJobSpecContinuesAStoppedJob = answer
	sem.KillJobSpecAimsAtTheGroup = No
	sem.JobsShowBackgroundCommand = Yes
	var err bytes.Buffer
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "testsh",
		Stdout: &err, Stderr: &err,
	})
	sent := []syscall.Signal{}
	r.SignalGroup = func(_ int, sig syscall.Signal) error {
		sent = append(sent, sig)
		return nil
	}
	r.addStoppedJob(pid, []string{"sleep", "300"}, syscall.SIGSTOP)
	return r, &sent, &err
}

// kills runs `kill` with the given arguments on that shell.
func kills(t *testing.T, r *Runner, args ...string) int {
	t.Helper()
	return biKill(r, context.Background(), args)
}

// The continue goes out, and it goes out **before** the signal the script
// asked for — which is the whole of the fix, because a stopped process holds
// a signal pending until something runs it again.
//
// Both answers in one table, because the axis is the subject: a test that
// only asked the Yes half would pass for an engine that continued in every
// dialect, and a test that only asked the No half would pass for the engine
// this replaced, which continued in none.
func TestWhetherAStoppedJobIsContinuedBeforeTheSignal(t *testing.T) {
	for _, tc := range []struct {
		why  string
		axis Answer
		want []syscall.Signal
	}{
		{"zsh continues it first", Yes, []syscall.Signal{syscall.SIGCONT, syscall.SIGTERM}},
		{"everybody else sends only what was asked for", No, []syscall.Signal{syscall.SIGTERM}},
	} {
		t.Run(tc.why, func(t *testing.T) {
			r, sent, out := killStoppedShell(t, tc.axis)
			if st := kills(t, r, "-TERM", "%1"); st != 0 {
				t.Fatalf("kill -TERM %%1: status %d, output %q", st, out.String())
			}
			if !sameSignals(*sent, tc.want) {
				t.Errorf("sent %v, want %v", *sent, tc.want)
			}
		})
	}
}

// And the table says the job is running afterwards, because it is. Without
// this a later `jobs` would call a process the shell has itself continued
// `suspended`, which is the reference's answer to neither half — zsh writes
// `running` there, measured on 5.9.2 the same day.
func TestTheContinuedJobIsNoLongerStopped(t *testing.T) {
	for _, tc := range []struct {
		why     string
		axis    Answer
		stopped bool
	}{
		{"continued, so it is running", Yes, false},
		{"not continued, so it is still stopped", No, true},
	} {
		t.Run(tc.why, func(t *testing.T) {
			// Signal 0, which delivers nothing at all: what moves the flag
			// is the continue and never the signal the script named.
			r, _, out := killStoppedShell(t, tc.axis)
			if st := kills(t, r, "-0", "%1"); st != 0 {
				t.Fatalf("kill -0 %%1: status %d, output %q", st, out.String())
			}
			if got := r.jobs[0].Stopped; got != tc.stopped {
				t.Errorf("Job.Stopped = %v, want %v", got, tc.stopped)
			}
		})
	}
}

// Signal 0 is the sharpest row of all and is the one the reference makes
// plainest: it performs the permission check and delivers nothing, so a
// shell that continues the job on it is continuing it because of the *spec*
// and not because of anything the signal could do. zsh's job reads `SN`
// afterwards; every other column's reads `T`.
func TestASignalThatDeliversNothingStillContinuesTheJob(t *testing.T) {
	r, sent, out := killStoppedShell(t, Yes)
	if st := kills(t, r, "-0", "%1"); st != 0 {
		t.Fatalf("kill -0 %%1: status %d, output %q", st, out.String())
	}
	if !sameSignals(*sent, []syscall.Signal{syscall.SIGCONT, syscall.Signal(0)}) {
		t.Errorf("sent %v, want a continue and then the nothing that was asked for", *sent)
	}
}

// Not for a signal that **stops**, which is the one shape the rule excludes:
// continuing a job in order to stop it would be a shell arguing with itself.
//
// Measured on zsh 5.9.2, the same session as the rows above: `kill -STOP %1`,
// `-TSTP`, `-TTIN` and `-TTOU` all leave a stopped job reading `TN`, where
// every other signal it was asked with — TERM, HUP, INT, KILL, QUIT, USR1,
// WINCH, URG, CHLD and 0 — leaves it `SN` or gone.
//
// The four are asserted beside a fifth that is not one of them, because a
// list that excluded everything would pass this just as readily.
func TestAStoppingSignalDoesNotContinueTheJobFirst(t *testing.T) {
	for _, tc := range []struct {
		spec string
		sig  syscall.Signal
		cont bool
	}{
		{"-STOP", syscall.SIGSTOP, false},
		{"-TSTP", syscall.SIGTSTP, false},
		{"-TTIN", syscall.SIGTTIN, false},
		{"-TTOU", syscall.SIGTTOU, false},
		{"-WINCH", syscall.SIGWINCH, true},
	} {
		t.Run(tc.spec, func(t *testing.T) {
			r, sent, out := killStoppedShell(t, Yes)
			if st := kills(t, r, tc.spec, "%1"); st != 0 {
				t.Fatalf("kill %s %%1: status %d, output %q", tc.spec, st, out.String())
			}
			want := []syscall.Signal{tc.sig}
			if tc.cont {
				want = []syscall.Signal{syscall.SIGCONT, tc.sig}
			}
			if !sameSignals(*sent, want) {
				t.Errorf("sent %v, want %v", *sent, want)
			}
			if got, wantStopped := r.jobs[0].Stopped, !tc.cont; got != wantStopped {
				t.Errorf("Job.Stopped = %v, want %v", got, wantStopped)
			}
		})
	}
}

// The `%` word is what decides, and not what the word resolves to.
//
// This is the noun the rule is keyed on, and the pair that pins it is the
// same job named two ways in the same second. Measured on zsh 5.9.2: `kill -0
// %1` on a stopped job leaves it `SN`, and `kill -0 "$!"` on that same job
// leaves it `TN`. So a number is a number here even though the shell knows
// perfectly well whose it is — which rules out "the target resolves to a
// job", the reading a probe that only ever wrote `%1` would have confirmed.
//
// The job's pid is this process's own, so that the number route is a real
// send the kernel answers rather than an ESRCH that would have recorded
// nothing whatever the rule was. Signal 0 delivers nothing, here as
// everywhere else in this file.
//
// Both spellings in one test, because the claim is the *difference* between
// them: the `%` row is what says the shell would have continued this very job
// had it been named the other way.
func TestAPidNamingTheSameJobIsNotContinued(t *testing.T) {
	self := os.Getpid()
	spec, _, out := killStoppedShellPID(t, Yes, self)
	if st := kills(t, spec, "-0", "%1"); st != 0 {
		t.Fatalf("kill -0 %%1: status %d, output %q", st, out.String())
	}
	if spec.jobs[0].Stopped {
		t.Fatal("the `%` spec did not continue the job, so the row below grades nothing")
	}

	r, sent, out := killStoppedShellPID(t, Yes, self)
	if st := kills(t, r, "-0", strconv.Itoa(self)); st != 0 {
		t.Fatalf("kill -0 %d: status %d, output %q", self, st, out.String())
	}
	if len(*sent) != 0 {
		t.Errorf("the group hook was asked for %v; a number goes to the process", *sent)
	}
	if !r.jobs[0].Stopped {
		t.Error("the job named by its pid was continued; only a `%` spec is")
	}
}

// And the same rule at the decision itself, which is where it can be shown
// rather than inferred.
//
// The route matters here and it is worth saying why this row is not simply a
// second spelling of the one above. Within this engine the only operand that
// is not a `%` word and still reaches a job is the identity invented for a
// job with **no process of its own** (jobident.go), and such a job has
// nothing to continue whatever the rule says — so every end-to-end assertion
// about the number route passes with the key and without it. Asking the
// decision directly is what makes the key falsifiable: the same job, the same
// signal, the same dialect, one word changed.
func TestOnlyAJobSpecReachesTheDecision(t *testing.T) {
	r, sent, _ := killStoppedShell(t, Yes)
	j := r.jobs[0]

	r.continueAStoppedJobSpec(j, false, syscall.SIGTERM)
	if len(*sent) != 0 || !j.Stopped {
		t.Errorf("a number continued the job: sent %v, stopped %v", *sent, j.Stopped)
	}
	// The positive half, on the same job in the same state: without it the
	// row above would pass for a decision that never continues anything.
	r.continueAStoppedJobSpec(j, true, syscall.SIGTERM)
	if !sameSignals(*sent, []syscall.Signal{syscall.SIGCONT}) || j.Stopped {
		t.Errorf("a `%%` spec did not continue the job: sent %v, stopped %v", *sent, j.Stopped)
	}
}

// A job that is already running is not continued either, which is what keeps
// the axis off every ordinary `kill %1`: the question is asked only where it
// decides something.
func TestARunningJobIsNotContinued(t *testing.T) {
	r, sent, out := killStoppedShell(t, Yes)
	r.jobs[0].Stopped = false
	if st := kills(t, r, "-TERM", "%1"); st != 0 {
		t.Fatalf("kill -TERM %%1: status %d, output %q", st, out.String())
	}
	if !sameSignals(*sent, []syscall.Signal{syscall.SIGTERM}) {
		t.Errorf("sent %v, want only the signal that was asked for", *sent)
	}
}

// And an unanswered axis refuses in the shipped binary rather than picking
// one shell's answer quietly.
func TestTheContinueAxisIsRefusedWhenNobodyAnsweredIt(t *testing.T) {
	r, sent, out := killStoppedShell(t, Unspecified)
	if st := kills(t, r, "-TERM", "%1"); st != 2 {
		t.Errorf("status %d, want 2", st)
	}
	const want = "`kill` continuing a stopped job named by a `%` spec"
	if !strings.Contains(out.String(), want) {
		t.Errorf("refusal was %q, want it to name %q", out.String(), want)
	}
	if len(*sent) != 0 {
		t.Errorf("sent %v, want nothing at all from a shell that could not answer", *sent)
	}
}

func sameSignals(got, want []syscall.Signal) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
