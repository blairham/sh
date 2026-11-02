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
// **A foreground pipeline and a `<(…)` are the same question with a split in
// each**, so each is a Semantics axis, asked only at a group kill from inside
// the body the shell itself forked under the monitor. Measured 2026-11-01 on
// a pseudo-terminal (`script -q /dev/null`), bash 5.3.20 and 3.2.57 and
// ksh93u+ also with no terminal and each shell in a group of its own, BusyBox
// ash 1.37.0 in the pinned alpine on docker's terminal:
//
//	set -m; trap 'echo T' USR1; : | (kill -USR1 0; echo in); echo out=$?
//	        bash, zsh, dash, ash: out=158 (138), no T; ksh93: T, out=286
//	…without `set -m`                      T in every column
//	…with the element first, `( … ) | /bin/cat`    the same split
//	set -m; : | (kill -USR1 0; echo in); echo out=$?
//	        bash, zsh, dash, ash: out=158; ksh93: the whole shell killed
//	set -m; : | { kill -USR1 -$BASHPID; echo in k=$?; }
//	        bash: no such process, in k=1 — a later element leads no group
//	set -m; { kill -USR1 -$BASHPID; echo in k=$?; } | /bin/cat
//	        bash: no T, out=0 — the first element leads the pipeline's
//	set -m; cat <( p=$BASHPID; kill -0 -- -$p; echo k=$? )
//	        bash: no such process, k=1; zsh (with $sysparams[pid]): k=0;
//	        ksh93 (the pid read back from a child): no such process
//	set -m; trap 'echo T' USR1; cat <(kill -USR1 0; echo in); echo out=$?
//	        bash: T, out=0; zsh: out=0, no T; ksh93: T, out=286
//
// So Semantics.MonitoredPipelineIsAGroupOfItsOwn (#7318) and
// Semantics.MonitoredProcessSubstitutionIsAGroupOfItsOwn (#7306). The noun is
// the same as above, the monitor at the fork of the body the shell forked,
// with **which body** added: a pipeline element is in the pipeline's group and
// only the first leads it, and a `<(…)` leads its own. A `$( … )` is neither,
// and is not asked: no shell forks it as a job (T in every column), though its
// own number under the monitor is #7324's.
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
	top := r.theBodyTheShellForked()
	if top == nil || !top.monitored || top.job == notAJob || !r.jobIsAGroupOfItsOwn(top) {
		return nil
	}
	return top
}

// theJobAGroupKillReaches is theJobAround for a kill aimed at pid, where pid
// is 0 or the group the job's body leads: nil for any other target, which
// asks the dialect nothing.
func (r *Runner) theJobAGroupKillReaches(pid int) *Process {
	if top := r.theBodyTheShellForked(); top == nil || pid != 0 && !top.anchor.leads(-pid) {
		return nil
	}
	return r.theJobAround()
}

// theBodyTheShellForked is the outermost body around this runner: the one the
// shell itself forked, which is the only one job control was on for.
func (r *Runner) theBodyTheShellForked() *Process {
	var top *Process
	for p := r.process; p != nil; p = p.outer {
		top = p
	}
	return top
}

// jobIsAGroupOfItsOwn reports that p, forked by the shell under the monitor,
// is in a process group the shell is not in. Asked, where the dialect decides.
func (r *Runner) jobIsAGroupOfItsOwn(p *Process) bool {
	switch p.job {
	case aJob:
		return true
	case aPipelineLeader, aPipelineElement:
		return r.ask(r.sem().MonitoredPipelineIsAGroupOfItsOwn,
			"whether a pipeline forked under the monitor is a process group of its own")
	case aProcessSubstitution:
		return r.ask(r.sem().MonitoredProcessSubstitutionIsAGroupOfItsOwn,
			"whether a process substitution forked under the monitor is a process group of its own")
	}
	return false
}

// jobLeadsItsGroup reports that p's own number names the group it is in: a
// job of its own does, and so does a pipeline's first element, while a later
// element is in the group the first one leads and never leads one itself.
func (r *Runner) jobLeadsItsGroup(p *Process) bool {
	return p.job != aPipelineElement && r.jobIsAGroupOfItsOwn(p)
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
