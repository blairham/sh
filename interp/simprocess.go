// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"sync"
	"sync/atomic"
)

// Process is the process a real shell would be running a body in, for a
// question that is asked *per process* and that this shell, which forks
// nothing, would otherwise answer once for all of them.
//
// The worked case is a record lock. A real shell's subshell is a fork, and a
// lock is the process's: a `( … )` does not inherit the lock its parent holds,
// cannot take a lock a sibling holds, and drops what it took when it exits.
// Measured against zsh 5.9.2 with `zsystem flock`: a shell holding a file and
// asking again is 0, and a subshell, a command substitution or a non-last
// pipeline element of that shell asking for it is `failed to lock file`.
// Here every one of those is a goroutine of one process, and the system's own
// lock — which is the process's — answers 0 to all of them.
//
// So a body that would have been forked gets a Process of its own, and the
// question is asked of that rather than of the system. Nil is a value: it is
// the shell itself, which has a real process and needs no stand-in for one,
// exactly as with bodyAnchor. Comparing two of these is comparing identities.
type Process struct {
	mu    sync.Mutex
	exits []func()
	done  bool

	// outer is the process of the body this one was forked from, and nil
	// where that is the shell. A process always has one, anchor or not,
	// which is why the bodies around a body are walked here rather than
	// through the anchors: those exist only where a front end supplied
	// ProcessAnchor, and a body is just as much inside another without one.
	// See Runner.bodiesAround.
	outer *Process
	// anchor is the group this body leads where it has been given one, and
	// what a copy of the body that aims at that group's number reaches. See
	// selfkillbody.go.
	anchor *procAnchor
	// arrivals are the signals copies of this body left it — at its number,
	// or at the process group every body around the sender is a member of —
	// waiting for its next moment between commands, and posted says there
	// are any, so the body can ask without taking the lock before every
	// command it runs.
	arrivals []bodyArrival
	posted   atomic.Bool

	// monitored records that the body was forked while the monitor was on,
	// which is when a real shell can have made it a group of its own.
	// Without it the group a placeholder leads is this shell's arrangement
	// and no reference's, and a script aiming a group kill at it is refused.
	// See aimedAtAGroupNoForkLeads. On the process rather than the anchor,
	// because a body is forked under the monitor or not whether or not a
	// front end supplied ProcessAnchor.
	monitored bool
	// job records that a real shell forks this body as a job of its own: a
	// `( … )` the dialect does not run in its own process, or a background
	// job's body. Forked by the shell itself under the monitor, such a body
	// leads a process group the shell is not in. See jobgroupsignal.go.
	job bool
}

// Process answers which process this runner's code would be running in. Nil
// is the shell's own; see the type.
func (r *Runner) Process() *Process {
	return r.process
}

// AtExit registers what has to happen when the process ends — what the
// kernel would have done for a real one, such as dropping its locks. On the
// shell's own process it does nothing, since that process really ends and
// the kernel does it. On one that has already ended it runs at once, so a
// body that ends while a registration is in flight leaves nothing behind.
func (p *Process) AtExit(f func()) {
	if p == nil {
		return
	}
	p.mu.Lock()
	if p.done {
		p.mu.Unlock()
		f()
		return
	}
	p.exits = append(p.exits, f)
	p.mu.Unlock()
}

// exit ends the process: everything registered runs, latest first, once.
func (p *Process) exit() {
	p.mu.Lock()
	if p.done {
		p.mu.Unlock()
		return
	}
	p.done = true
	exits := p.exits
	p.exits = nil
	p.mu.Unlock()
	for i := len(exits) - 1; i >= 0; i-- {
		exits[i]()
	}
}

// ended reports that the process has ended. The shell's own, nil, has not.
func (p *Process) ended() bool {
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.done
}

// forkProcess gives a cloned runner a process of its own and answers with
// what ends it. Called beside the anchor at each of the five places a real
// shell forks; see Runner.anchorForkedBody.
func (c *Runner) forkProcess() func() {
	// The process the clone inherited is the one it was forked from, and
	// the clone has not run anything yet, so its monitor is still the one of
	// the shell that forked it.
	p := &Process{outer: c.process, monitored: c.monitor}
	c.process = p
	return p.exit
}
