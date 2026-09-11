// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"os"
	"sync"
)

// A process substitution's body runs on a goroutine of this process, and some
// of what a script asks of such a body has no answer without a process. This
// file gives it the smallest real one: a **process group**, led by a placeholder
// child that exists to be its leader and does nothing else.
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
// The anchor is started at the first read and never otherwise. A `<(…)` whose
// body never asks which process it is costs nothing, which is every one in the
// corpus and all but four in the plugin tree — and the four that ask are
// asking for something worth a process.
//
// # What ends it
//
// The same count that decides when the substitution's own pipe end may be
// closed: substEnd. A real shell's fork gives the pipe and the process group
// one lifetime, and it is the family's rather than the body's — a job the body
// backgrounded holds both open after the body has returned, which is the whole
// reason that count exists. Reconstructing the group's lifetime is the same
// problem as reconstructing the descriptor's, so it is the same answer and not
// a second one beside it.

// procAnchor is the placeholder process that leads a substitution body's
// process group, and the group's id.
//
// A nil *procAnchor is a body that cannot have one — no front end supplied the
// command, or the platform has no process groups — and every method answers
// for that case, so no caller has to.
type procAnchor struct {
	argv []string

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
		return 0, false
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
// It answers only for a process substitution's body, which is where the
// question is asked in the wild and the one place this shell already
// reconstructs that body's lifetime — see substEnd. A subshell, a command
// substitution and a background job have no group of their own yet and answer
// false, which is what every body answered before this existed.
//
// A dialect asks it where it would otherwise have to answer "which process am
// I" with a number that is not one. The answer is deliberately the *group*:
// see procanchor.go for why that is the reading the scripts asking for it
// spend it under, and why this shell's own number is the one answer that must
// never be given.
func (r *Runner) SubshellProcessGroup() (int, bool) {
	if r == nil || r.pipeEnd == nil {
		return 0, false
	}
	return r.pipeEnd.anchor.group()
}

// anchoredGroup is the group a command this runner starts should join: the one
// the body already asked for, and nothing otherwise.
func (r *Runner) anchoredGroup() (int, bool) {
	if r.pipeEnd == nil {
		return 0, false
	}
	return r.pipeEnd.anchor.existing()
}
