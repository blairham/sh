// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"sync"
	"sync/atomic"
)

// A background job here is a goroutine, and some of them are only that. An
// external command gives the job a real process and a real process id; a
// background builtin, a backgrounded compound command whose body never reaches
// an external program, and a function that only sets variables give it none.
// This file is the number those jobs answer to.
//
// # Why they have to answer at all
//
// `$!` is the most recent background job, and every shell in the panel reports
// a process id there whatever the job was made of. Measured 2026-09-13 on
// `( exit 5 ) & a=$!`, `true & b=$!`, `{ :; } & c=$!` and `/bin/sleep 0 & d=$!`
// in bash 5.3.15, that same bash invoked as `sh`, bash 3.2.57, zsh 5.9.2,
// ksh93u+ 2012-08-01, dash and BusyBox ash 1.37.0: all seven report a positive
// number for every one of the four, and all four numbers differ. They fork
// before the body runs a thing, so the question never comes up for them.
//
// This shell reported 0 for the three that never reach a program, and 0 is not
// a spare value. POSIX gives it to `kill` as *every process in the sender's
// process group*, so `p=$!; kill "$p"` — the line a script writes to stop one
// job — was a line aimed at the shell and everything it had started (#2650).
// The two jobs were not distinguishable from each other either:
// `( exit 5 ) & a=$!; ( exit 6 ) & b=$!; wait "$a"` answered 6, because both
// reads were 0 and 0 matched whichever job the walk reached last.
//
// # What the number is
//
// A number this shell invents, and one the kernel cannot have issued. Every
// Unix caps process ids well below 2^22 — Linux's `pid_max` cannot be set
// higher than 4194304 on a 64-bit machine, and macOS and the BSDs stop at
// 99999 — so a number at 2^30 names no process anywhere, and that is the whole
// point of putting it there rather than at 1: a script that spends `$!` on
// something outside this shell must reach *nothing* rather than something that
// happens to be running. `/bin/kill "$!"` on such a job fails with no such
// process, where it used to name a process group.
//
// Inside the shell it is not a guess at all. `wait` and `kill` look the number
// up in the job table before they go anywhere near the kernel, so `wait "$!"`
// reports the job's status and `kill "$!"` reaches the job's processes — which
// for a job made only of builtins is none, and is reported as the job with
// nothing to signal that `kill %1` already reports.
//
// The deviation is recorded rather than hidden: a real shell's `$!` is a
// process that exists and can be found in `ps`, and this one cannot. See
// docs/spec/semantics.md.
//
// # Why it is not a pid, decided rather than outstanding
//
// #4480 asked for one, on the ground that a number outside this shell reaches
// nothing: `kill -0 "$!"` from a sibling process, a pid written to a file, a
// supervisor reading it. All three are true and none of them is reachable
// from here.
//
// A real shell answers `: &` with a pid because it **forks**, and the fork is
// the whole of the answer: the child is a copy of the shell, so the builtin
// runs in a process of its own with nothing to arrange. Go has no such call —
// only fork-and-exec — so this shell cannot produce a process that is a copy
// of itself, and a Runner is embedded in other programs besides, where
// starting a process for `: &` is exactly the thing the core may not do (see
// the rule in AGENTS.md, and interp/runner.go on `exec` and `kill`).
//
// The one thing that *would* give a real number is a placeholder process kept
// alive for the job's lifetime, and that is worse than the number here rather
// than better: it exists, so `kill "$!"` reaches it and reports success, and
// the job it was supposed to name goes on running. A number that reaches
// nothing fails honestly.
//
// The measurement the issue rests on is also not quite right, and it is worth
// writing down because it reads as an argument for a constant: the value is a
// counter and not a constant. `: & print $!` three times over answers
// 1073741825, 1073741826 and 1073741827 — measured 2026-09-26 on cmd/zsh —
// so two such jobs are distinguishable from each other, which is the property
// #2650 was about. What is fixed is the *base*, and it is fixed on purpose.

// inventedJobIdentBase is where invented job identities start: above every
// process id any Unix can issue, for the reason given above.
const inventedJobIdentBase = 1 << 30

// inventJobIdent is the next such number for this shell.
//
// **Per shell rather than per process, and shared down the clone chain.** A
// counter of one shell's own is what makes the value reproducible: a script
// that starts its jobs one after another hands out the same numbers every
// run, where a process-wide counter hands out different ones on the second
// run and puts a value in the output that is not the same twice. This
// repository forbids exactly that — see interp/printbehaviour_test.go, which
// runs a case and then runs its printed form and compares, and which a
// drifting counter failed.
//
// Two jobs started *concurrently* — a background job that itself backgrounds
// something — draw in whichever order the scheduler gives, so they are
// distinct rather than fixed. That is as much as a real shell offers for the
// same shape, and it is why nothing prints the value: the corpus compares
// `$!` readings against each other and never writes one down.
//
// Shared down the chain because that is where a collision could come from: a
// subshell inherits its parent's job table, so a number it invented for a job
// of its own must not be a number one of those already answers to. Runner.clone
// copies the pointer, so every shell descended from one root draws from one
// counter. Two shells that never share a table may share a number and it
// reaches nothing — a child's jobs are never in a parent's table.
//
// Allocated at the first read rather than at setup, which is the same choice
// and the same reason as every other lazily built table here: a script that
// backgrounds nothing costs one nil check. It is read only on the shell's own
// goroutine, before the clone the job will run on exists.
//
// Masked back into range rather than allowed to climb, so that the result
// always fits an int on a 32-bit platform. A session would have to start a
// billion processless background jobs to reach the wrap, and at that point the
// oldest of them is long gone.
func (r *Runner) inventJobIdent() int {
	if r.jobIdents == nil {
		r.jobIdents = new(atomic.Uint32)
	}
	return inventedJobIdentBase + int(r.jobIdents.Add(1)&(inventedJobIdentBase-1))
}

// inventedJobRegistry is every job this shell and its descendants started that
// answers to an invented number, for the `kill` that names one by number from
// a shell whose job table does not list it.
//
// A real shell's `$!` is a process id, and a process id reaches the process
// from anywhere: a background body that was handed `$p` signals the job its
// parent started, though its own `jobs` lists nothing. Measured 2026-10-03 on
// ksh93u+ 2012-08-01, whose `sleep` is a builtin, with `sleep 0.3 & p=$!`:
// `{ kill -0 $p && echo alive; } &` prints alive, and `{ kill $p; } & wait
// $p` terminates the job. Here the number named nothing outside the table, so
// both said `kill: 1073741825: no such process` (#5684). Only `kill` reads
// this: a `wait` for a job that is not this shell's child is refused in every
// column, so the table stays its answer.
type inventedJobRegistry struct {
	mu   sync.Mutex
	jobs map[int]*Job
}

// registerInventedJob records a job by the number it was invented, dropping
// any that have finished.
func (r *Runner) registerInventedJob(j *Job) {
	if r.inventedJobs == nil {
		r.inventedJobs = &inventedJobRegistry{}
	}
	reg := r.inventedJobs
	reg.mu.Lock()
	defer reg.mu.Unlock()
	if reg.jobs == nil {
		reg.jobs = map[int]*Job{}
	}
	for n, k := range reg.jobs {
		if k.Finished() {
			delete(reg.jobs, n)
		}
	}
	reg.jobs[j.ident] = j
}

// reachInventedJob is the running job an invented number names anywhere in
// this shell's family, and still answers to: a job that has since started a
// process answers to that process instead. A finished one is left to its own
// shell's table, which keeps it answering until a `wait` collects it and
// then lets it go (TestAnEndedJobStillAnswersKillUntilItIsWaitedFor).
func (r *Runner) reachInventedJob(n int) *Job {
	if n < inventedJobIdentBase || r.inventedJobs == nil {
		return nil
	}
	reg := r.inventedJobs
	reg.mu.Lock()
	j := reg.jobs[n]
	reg.mu.Unlock()
	if j == nil || j.Finished() || j.Ident() != n {
		return nil
	}
	return j
}
