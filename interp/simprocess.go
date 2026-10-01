// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "sync"

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

// forkProcess gives a cloned runner a process of its own and answers with
// what ends it. Called beside the anchor at each of the five places a real
// shell forks; see Runner.anchorForkedBody.
func (c *Runner) forkProcess() func() {
	p := &Process{}
	c.process = p
	return p.exit
}
