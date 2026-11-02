// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "syscall"

// Under the monitor, a body a real shell forks **as a job** — a `( … )`, a
// background job's body — leads a process group of its own, and the shell is
// not in it. So `kill -SIG 0` inside it reaches that group alone: the body and
// every body inside it die of the signal, and the shell, which never had a
// copy, runs no handler. Measured 2026-11-01 under `-c`, each shell in a
// process group of its own and no terminal, against bash 5.3.20 and bash
// 3.2.57:
//
//	set -m; trap 'echo T' USR1; (kill -USR1 0; echo in); echo out=$?
//	        the notice, out=158, no T; here, before: T as well
//	set -m; (kill -USR1 0; echo in); echo out=$?    and TERM
//	        the notice, out=158 (143); here, before: nothing — the
//	        kernel's copy killed the whole shell
//	set -m; trap 'echo T' USR1; ( (kill -USR1 0; echo in2); echo in1 )
//	        the notice for the outer parentheses, out=158, no T
//	set -m; trap 'echo T' USR1; (kill -USR1 0; echo in) & wait $!
//	        out=158, no T
//	set -m; trap 'echo T' USR1; (kill -USR1 -$BASHPID; echo in)
//	        the first row: the body's own number names the same group;
//	        here, before: in, out=0 — the placeholder died instead
//	set -m; trap 'echo T' USR1; (kill -USR1 -$$; echo in)
//	        in, T, out=0: the shell's group, which the body is not in;
//	        here, before: the notice, T, out=158
//
// zsh 5.9.2 refuses `set -m` without a terminal and dash turns it off, so on
// a pseudo-terminal (`script -q /dev/null`), where both grant it: zsh and dash
// write every row above as bash does. ksh93u+ runs a `( … )` in its own
// process, so there is no fork to be a job, and it writes T and out=286 with
// the monitor as without it; its background job is a group of its own and
// writes no T. So every shell that forks agrees, and it is not a dialect
// split.
//
// The noun is **the monitor at the fork of the body the shell itself
// forked.** It is not the monitor at the `kill`: `trap 'echo T' USR1; (set
// -m; kill -USR1 0; echo in)` writes T in bash, because the parentheses were
// forked without it and are in the shell's group, and `set -m; trap …; (set
// +m; kill -USR1 0; echo in)` writes none — #7238's pair, and the record it
// keeps (Process.monitored). And it is only the outermost body's: a body
// forked inside a job has no job control of its own, so the third row takes
// both parentheses and the shell writes no T, in bash, zsh and dash alike on
// a terminal.
//
// Not a pipeline element yet. bash, zsh and dash give a foreground pipeline
// a group of its own under the monitor, and ksh93 leaves its elements in the
// shell's group under `-c` even on a terminal, which is a split for an axis
// to answer (#7318). Nor `$( … )`, which bash does not fork as a job (T in
// every column), nor `<(…)`, which is #7306's.
//
// **Nothing goes to the kernel.** Not the shell's group: that is this
// process, which is the shell, and is what killed the whole shell for an
// untrapped signal. Nor the group the job's placeholder leads, where the body
// asked for its number and has one: the placeholder would die of it, and a
// program the body was starting into that group a moment later failed with
// `operation not permitted` — measured, every time, on #7306's `set -m; (
// p=$BASHPID; { kill -- -$p; echo k=$?; } | /bin/cat; /bin/echo after )`,
// which without the send writes bash's notice and `outer 143` and nothing
// else. What that gives up is a program already running in that group, which
// a real shell's group kill would reach and this one leaves to end on its own:
// the race #7238 took out of the shape without the monitor, not reintroduced
// with it. The bodies settle their copies at their next moment, as a copy of a
// body settles one aimed at the body's number (selfkillbody.go), innermost
// outward.

// theJobAround is the job whose process group this runner is in rather than
// the shell's: the outermost body around it — the one the shell itself forked
// — where a real shell forked that as a job with the monitor on. Nil
// otherwise, which is every body in a shell without the monitor.
func (r *Runner) theJobAround() *Process {
	var top *Process
	for p := r.process; p != nil; p = p.outer {
		top = p
	}
	if top == nil || !top.job || !top.monitored {
		return nil
	}
	return top
}

// signalTheJobsGroup answers a signal this runner sent the group of the job it
// is in: its own copy, and the copy of every body around it out to the job's.
// The shell has none, and the bodies are every member this shell can answer
// for: the job is the outermost of them.
func (r *Runner) signalTheJobsGroup(name string, sig syscall.Signal) error {
	_ = r.signalThisBody(name, sig)
	for p := range r.bodiesAround {
		r.signalTheBodyAround(p, name, sig)
	}
	return nil
}
