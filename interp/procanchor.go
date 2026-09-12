// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"os"
	"sync"
)

// A body a real shell would have *forked* runs on a goroutine of this process
// here — a subshell, a command substitution, a pipeline element, a background
// job or a process substitution's body — and some of what a script asks of
// such a body has no answer without a process. This file gives it the smallest
// real one: a **process group**, led by a placeholder child that exists to be
// its leader and does nothing else.
//
// # What asks for it
//
// A script that starts a daemon through `<(…)` and wants to be able to stop it
// again. The shape is the same in all four instances of it installed on this
// machine — a prompt theme's asynchronous worker, its git backend, an
// autosuggestion plugin and a syntax highlighter — and it is three lines long:
//
//	sysopen -r -u fd <(
//	  local pgid=$sysparams[pid]   # which process am I?
//	  … start the daemon …
//	  kill -- -$pgid               # and tear the whole group down
//	)
//
// The value is read as an identity and spent as a process group. That is why
// answering it with *this* process's number is the one thing that must not
// happen: the body is the shell, so `kill -- -$pgid` is the interactive
// shell's own group, and a session that reached its first prompt took its own
// SIGTERM a third of a second later (#2046). Answering nothing is safe and is
// what this shell did instead — and a body that needs a process of its own
// then declines to start, silently, which is what a prompt reporting
// `gitstatus failed to initialize` on every start was (#2083).
//
// # Why a group rather than a process
//
// Neither of those two answers is available, but the *question* the script
// asks has a third one. Read the three lines again: nothing in them needs the
// body to be a process. They need a number that
//
//   - is real, so a numeric test passes and a signal can be aimed at it,
//   - names everything that body starts, so the teardown reaches the daemon,
//   - and names nothing else, so the teardown reaches nothing of the shell's.
//
// A process group is exactly that, and this shell can have one honestly: put
// the body's children in a group of their own and hand the script its id. The
// only thing missing is a leader, because a process group cannot be created
// without a process in it — so one is started, it does nothing, and it exits
// when the body's family is done.
//
// So the value this shell gives is not "the process I am". It is "the group I
// lead", which is the reading the script actually spends it under, and the
// deviation is recorded where the parameter is answered rather than hidden
// here. See dialect/zsh's subshellPid.
//
// # Lazily, and only for a body that asks
//
// The anchor is started at the first read and never otherwise. A body that
// never asks which process it is costs one nil pointer, which is every body in
// the corpus and all but five in the plugin tree — and the five that ask are
// asking for something worth a process. That is what makes it affordable to
// give every forked body one: a loop of subshells forks nothing.
//
// # What ends it
//
// Each body's own, and this is the whole of what differs between the five.
//
// A process substitution's is the count that decides when its pipe end may be
// closed: substEnd. A real shell's fork gives the pipe and the process group
// one lifetime, and it is the *family's* rather than the body's — a job the
// body backgrounded holds both open after the body has returned, which is the
// whole reason that count exists. Reconstructing the group's lifetime is the
// same problem as reconstructing the descriptor's, so it is the same answer
// and not a second one beside it.
//
// The other four are simpler and were named in #2114 before they were built:
// a subshell's and a command substitution's is the body's own run, because the
// caller joins before it carries on; a pipeline element's is the element's;
// and a background job's is the job's. Four release points, one field and one
// start — see Runner.anchorForkedBody for why that is one mechanism rather
// than four.

// procAnchor is the placeholder process that leads a substitution body's
// process group, and the group's id.
//
// A nil *procAnchor is a body that cannot have one — no front end supplied the
// command, or the platform has no process groups — and every method answers
// for that case, so no caller has to.
type procAnchor struct {
	argv []string
	// outer is the anchor of the body this one is nested in, and is what
	// `existing` falls back to. A body that never asks which process it is
	// has no group of its own, and a command it starts must then join the
	// group of the body around it rather than none at all: a job
	// backgrounded inside a `<(…)` whose own body asked is exactly the case
	// the substitution's teardown has to reach. Only `existing` walks it —
	// asking *which process am I* is answered by this body or not at all,
	// because a real shell forked again for it.
	outer *procAnchor

	mu sync.Mutex
	// tried records that the start has been attempted, so a failure is not
	// retried at every read. A body that could not have a group once cannot
	// have one a moment later, and a script that reads the parameter in a
	// loop must not fork a process per read.
	tried bool
	pgid  int
	// stdin is the write end of the pipe the anchor is reading. Closing it is
	// the end-of-file that ends the anchor, and is the only thing that does:
	// a signal aimed at the group by the script itself is the script's
	// business and reaps through the same wait.
	stdin *os.File
	// gone records that the anchor has been waited for, so that a child is
	// never asked to join a group whose leader has already exited and which
	// may hold nothing at all.
	gone bool
}

// newProcAnchor makes the anchor for one substitution body, unstarted.
func newProcAnchor(argv []string) *procAnchor {
	if len(argv) == 0 {
		return nil
	}
	return &procAnchor{argv: argv}
}

// group is the body's process group, started if this is the first read.
func (a *procAnchor) group() (int, bool) {
	if a == nil {
		return 0, false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.tried {
		return a.pgid, a.pgid > 0
	}
	a.tried = true
	cmd, stdin, err := startProcAnchor(a.argv)
	if err != nil {
		// Nothing to report: the parameter that asked answers as it did
		// before there was an anchor at all, which is the behavior every
		// caller already handles. A diagnostic here would land in the middle
		// of a prompt for something the script never asked this shell to do.
		return 0, false
	}
	a.pgid, a.stdin = cmd.Process.Pid, stdin
	// Waited for on a goroutine, because nothing else will: this child is not
	// a job, is not `$!`, and is not a command any script named. The wait is
	// also what tells the exec path the group has gone.
	go func() {
		_ = cmd.Wait()
		a.mu.Lock()
		a.gone = true
		a.mu.Unlock()
	}()
	return a.pgid, true
}

// existing is the group only if there already is one, so that starting a
// command never starts an anchor. A command the body runs joins the group the
// body asked for; it does not ask for one on the body's behalf.
func (a *procAnchor) existing() (int, bool) {
	if a == nil {
		return 0, false
	}
	a.mu.Lock()
	pgid, gone := a.pgid, a.gone
	a.mu.Unlock()
	if pgid <= 0 || gone {
		return a.outer.existing()
	}
	// And still there, which is a separate question from whether this shell
	// let it go: the script may have signaled the group itself. See
	// procGroupExists.
	if !procGroupExists(pgid) {
		return 0, false
	}
	return pgid, true
}

// stop lets the anchor go, by delivering the end-of-file it is reading for.
//
// Not a signal: the anchor's whole contract is that it exits when its standard
// input ends, and a shell that killed it instead would be signaling a group a
// script may still be holding the id of.
func (a *procAnchor) stop() {
	if a == nil {
		return
	}
	a.mu.Lock()
	f := a.stdin
	a.stdin = nil
	a.mu.Unlock()
	if f != nil {
		_ = f.Close()
	}
}

// SubshellProcessGroup is the process group a body a real shell would have
// forked leads here, started on demand.
//
// It answers for all five such bodies — a subshell, a command substitution, a
// pipeline element, a background job and a process substitution's body — and
// for none of them until one of them reads it. A shell that is not inside a
// body at all answers false and keeps its own process number, which is the one
// place `$sysparams[pid]` and `$$` agree in a real shell.
//
// A dialect asks it where it would otherwise have to answer "which process am
// I" with a number that is not one. The answer is deliberately the *group*:
// see the file comment above for why that is the reading the scripts asking
// for it spend it under, and why this shell's own number is the one answer
// that must never be given.
func (r *Runner) SubshellProcessGroup() (int, bool) {
	if r == nil {
		return 0, false
	}
	return r.bodyAnchor.group()
}

// anchoredGroup is the group a command this runner starts should join: the one
// the body already asked for, and nothing otherwise.
func (r *Runner) anchoredGroup() (int, bool) {
	return r.bodyAnchor.existing()
}

// anchorForkedBody gives a cloned runner the group a real shell's fork would
// have given it, and answers with what ends it.
//
// **One mechanism for the four bodies that had none**, which is the point of
// the function existing at all rather than four fields. A process
// substitution's body already had one, hung on the count that reconstructs its
// family's lifetime (substEnd); the other four each have a lifetime of their
// own — a subshell's and a command substitution's is the body's own run,
// because the caller joins before carrying on; a pipeline element's is the
// element's; and a background job's is the job's. What differs between them is
// *when the release is called*, and nothing else, so what they share is a
// field and a start rather than four helpers that will drift apart. The
// standing rule here is that a second helper spreads the bug.
//
// Nothing is started by this call. The anchor is lazy — a process is forked at
// the first read of the parameter and never otherwise — so every body in every
// script that does not ask which process it is costs one nil pointer.
//
// Called on the clone, before it runs, and its release must be called however
// the body ends. Overwrites whatever the clone inherited from the body around
// it, which is what a nested body wants: a real shell forks again there.
func (c *Runner) anchorForkedBody() func() {
	a := newProcAnchor(c.ProcessAnchor)
	if a == nil {
		// No placeholder to lead a group with, so there is nothing to
		// overwrite and nothing to release. The inherited field is nil for
		// the same reason: the program is the Runner's and the clone has the
		// same one.
		return func() {}
	}
	a.outer = c.bodyAnchor
	c.bodyAnchor = a
	return a.stop
}
