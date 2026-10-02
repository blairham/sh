// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"maps"
	"sync"
	"syscall"
)

// A signal the shell aims at one of its own background jobs, where the job is
// a body this shell runs rather than a process it started.
//
// A real shell forks for `{ … } &`, so the job is a process with a trap table
// of its own and `kill` reaches that table. Here the body is a goroutine on a
// cloned runner, and the only processes `kill` can find for it are the
// programs the body has started — which have none of the body's traps. So a
// TERM the body had trapped killed its `sleep` instead and the handler never
// ran. Measured 2026-10-02 on zsh 5.9.2, under `-f -c`:
//
//	{ trap 'print T' TERM; sleep 2; print after } & sleep 1; kill -TERM $!; wait
//	        T, then after — both two seconds in: the handler runs once the
//	        body's command is done, and the `sleep` is not interrupted
//	fn2() { trap 'print T; return 1' TERM; sleep 2 }; fn2 & sleep 1; kill -TERM $!; wait
//	        T
//	( trap 'print T' TERM; sleep 2; print after ) & …   T, then after
//
// The inbox is how the arrival crosses. It is written by `kill` on the
// shell's goroutine and read by the body between its commands, on the body's,
// so unlike Runner.selfPending it is locked. It also holds a copy of what the
// body has trapped, kept by the body as it sets and resets its traps, because
// `kill` has to decide *before* sending whether the body answers the signal —
// and the body's own table is the body goroutine's to read.
//
// Only a signal the body traps or ignores is taken here. One it leaves at its
// default goes to the body's processes as it always has: that is not what a
// forked shell does — the fork dies and its child runs on — but a goroutine
// cannot be ended in the middle of a wait, and that difference is not this
// file's to close.
type bodyInbox struct {
	mu      sync.Mutex
	traps   map[string]string
	pending []string
}

// newBodyInbox makes the inbox for a body starting with the trap table it
// was handed.
func newBodyInbox(traps map[string]string) *bodyInbox {
	return &bodyInbox{traps: maps.Clone(traps)}
}

// noteTrap is the body telling its inbox what a condition holds now.
func (b *bodyInbox) noteTrap(name string, body *string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if body == nil {
		delete(b.traps, name)
		return
	}
	if b.traps == nil {
		b.traps = map[string]string{}
	}
	b.traps[name] = *body
}

// deliver takes a signal for the body where the body answers it, and reports
// whether it did. A handled one is queued for the body's next moment between
// commands; an ignored one is taken and nothing is queued.
func (b *bodyInbox) deliver(key string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	action, ok := b.traps[key]
	if !ok {
		return false
	}
	if action != "" {
		b.pending = append(b.pending, key)
	}
	return true
}

// take hands the body what has arrived for it, and forgets it.
func (b *bodyInbox) take() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	got := b.pending
	b.pending = nil
	return got
}

// deliverToABody is `kill`'s half: the signal goes to the body of the job the
// operand names, where that job is one of these bodies and its traps answer
// the signal. named is the job the operand resolved to, if it did; a bare
// pid is matched against the jobs as well, because `$!` for a body that has
// started a program is that program's pid.
func (r *Runner) deliverToABody(named *Job, operand []jobProcess, name string, sig syscall.Signal) bool {
	if sig == 0 || !catchableSignal(sig) {
		return false
	}
	j := named
	if j == nil && len(operand) == 1 {
		j = r.runningJobWithPid(operand[0].pid)
	}
	if j == nil || j.inbox == nil || j.Finished() {
		return false
	}
	if !j.inbox.deliver(trapKey(name, sig)) {
		return false
	}
	s := r.sigs()
	s.mu.Lock()
	s.poke()
	s.mu.Unlock()
	return true
}

// runningJobWithPid is the job whose `$!` is pid, among those still running
// whose pid has settled.
func (r *Runner) runningJobWithPid(pid int) *Job {
	if pid <= 0 {
		return nil
	}
	for _, j := range r.jobs {
		select {
		case <-j.ready:
		default:
			continue
		}
		if j.PID == pid && !j.Finished() {
			return j
		}
	}
	return nil
}
