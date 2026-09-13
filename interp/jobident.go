// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "sync/atomic"

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

// inventedJobIdentBase is where invented job identities start: above every
// process id any Unix can issue, for the reason given above.
const inventedJobIdentBase = 1 << 30

// inventedJobIdents hands out the numbers. Process-wide rather than per
// Runner, so that a job started by a subshell cannot be handed a number a job
// of the shell around it already answers to — the two tables meet whenever a
// job is inherited, and two jobs with one number is the defect this is fixing
// rather than a smaller version of it.
var inventedJobIdents atomic.Uint32

// inventJobIdent is the next such number.
//
// Masked back into range rather than allowed to climb, so that the result
// always fits an int on a 32-bit platform. A session would have to start a
// billion processless background jobs to reach the wrap, and at that point the
// oldest of them is long gone.
func inventJobIdent() int {
	return inventedJobIdentBase + int(inventedJobIdents.Add(1)&(inventedJobIdentBase-1))
}
