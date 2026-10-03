// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"maps"
	"sync"
	"syscall"

	"github.com/blairham/sh/syntax"
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
// A signal the body leaves at a default that ends a process ends the *body*,
// and its programs run on: the fork dies and its child is orphaned. Measured
// 2026-10-02 on zsh 5.9.2 under `-f -c` (#5355):
//
//	{ sleep 1; print after } & sleep 0.2; kill -TERM $!; wait $!; print $?
//	        143, and nothing else — the `sleep` is still running afterwards
//	{ trap 'print X' EXIT; sleep 1; print after } & … kill -TERM $!   143,
//	        and no X: a process killed by a signal runs no EXIT trap
//	f & …, ( … ) & …, kill -TERM %1                                    143
//	{ sleep 1; print after } & … kill -USR1 $!; wait $!                158
//	{ trap 'print T' TERM; while :; do :; done } & … kill -KILL $!     ends
//
// The body cannot be stopped in the middle of whatever it is doing, so the
// death is recorded here and the body dies of it at its next moment between
// commands — and a wait for one of its programs is abandoned on the spot,
// without the program being signaled, which is what makes "the next moment"
// arrive at once rather than when the program ends. See
// Runner.waitForBackgroundProcess, which is where a body's programs are
// waited for; a body with the monitor on waits through the front end and is
// not given up yet.
//
// **Except where the fork would have become the program.** A real shell runs
// the last command of a forked body by exec'ing it, so the job *is* that
// program by then and the signal reaches it. Measured in the same run, with a
// probe for the program afterwards: `sleep &`, `{ sleep } &`, `( sleep ) &`,
// `{ true; sleep } &`, `if true; then sleep; fi &`, a `case` arm, `true &&
// sleep &`, `! sleep &`, `x=1 sleep &`, `command sleep &` and `exec sleep &`
// all leave no `sleep` behind, at 143; `f &` with a function whose body is
// the `sleep`, `{ sleep && true } &` and a body that has set an EXIT trap
// leave it running. See tailCommands.
//
// **A pipeline or a `( … )` the body is running is abandoned too** (#5386).
// Their elements are clones on goroutines of their own rather than a wait
// this can give up, so the job is ended from outside the body the moment the
// signal arrives — see Runner.abandonOnDeath — and the body's goroutine runs
// on to its next moment between commands, where it dies without a word, as
// the orphans of a real fork run on.
//
// Not INT or QUIT: a background job in a shell without job control ignores
// both, measured — `{ sleep 1; print after } & kill -INT $!` prints `after`.
// And not a signal whose default is to stop, continue or be ignored, which
// would end nothing in a fork either; those go to the body's processes as
// they always have.
type bodyInbox struct {
	mu      sync.Mutex
	traps   map[string]string
	pending []string
	// tails are the commands a fork would exec rather than run, and execd
	// says the body has reached one of them as a program. Both are the
	// exception above.
	tails map[*syntax.SimpleCmd]bool
	execd bool
	// fatal is the signal the body is to die of, and fatalName its name;
	// died is closed when fatal is first set, so a wait can select on it.
	fatal     syscall.Signal
	fatalName string
	died      chan struct{}
}

// newBodyInbox makes the inbox for a body starting with the trap table it
// was handed.
func newBodyInbox(traps map[string]string) *bodyInbox {
	return &bodyInbox{traps: maps.Clone(traps), died: make(chan struct{})}
}

// kill records the signal the body is to die of. The first one wins: a body
// that has been killed is not killed again by a second signal on its way out.
func (b *bodyInbox) kill(name string, sig syscall.Signal) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.fatal != 0 {
		return
	}
	b.fatal, b.fatalName = sig, name
	close(b.died)
}

// death is the signal the body is to die of, if one has arrived.
func (b *bodyInbox) death() (string, syscall.Signal) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.fatalName, b.fatal
}

// endsABodyByDefault reports whether a signal left at its default ends the
// body it is aimed at. See bodyInbox for the measurement and for why INT and
// QUIT are not among them.
func endsABodyByDefault(name string) bool {
	switch name {
	case "INT", "QUIT", "CHLD", "CLD", "CONT", "STOP", "TSTP", "TTIN", "TTOU",
		"URG", "WINCH", "IO", "POLL", "INFO":
		return false
	}
	return true
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

// tailCommands collects the simple commands a fork would exec as the last
// thing it does: the end of an `&&`/`||` list and of a pipeline of one, and
// through the bodies a fork runs straight through — a brace group, a `( … )`,
// each branch of an `if` and each arm of a `case`. A function call is a tail
// as a command but runs its body in the fork, so nothing inside it is one;
// it never reaches a program by this node, which is what keeps it out.
func tailCommands(e syntax.Expr, into map[*syntax.SimpleCmd]bool) {
	switch x := e.(type) {
	case *syntax.BinaryExpr:
		tailCommands(x.Y, into)
	case *syntax.Pipeline:
		if len(x.Cmds) == 1 {
			tailCommand(x.Cmds[0], into)
		}
	}
}

func tailCommand(c syntax.Command, into map[*syntax.SimpleCmd]bool) {
	switch x := c.(type) {
	case *syntax.SimpleCmd:
		into[x] = true
	case *syntax.Group:
		tailOfList(x.List, into)
	case *syntax.Subshell:
		tailOfList(x.List, into)
	case *syntax.IfClause:
		tailOfList(x.Then, into)
		for _, e := range x.Elifs {
			tailOfList(e.Then, into)
		}
		tailOfList(x.Else, into)
	case *syntax.CaseClause:
		for _, it := range x.Items {
			tailOfList(it.Body, into)
		}
	}
}

func tailOfList(list []*syntax.Stmt, into map[*syntax.SimpleCmd]bool) {
	if len(list) == 0 || list[len(list)-1].Background {
		return
	}
	tailCommands(list[len(list)-1].Expr, into)
}

// reachedATail notes that the body is running c as a program, where c is a
// command a fork would have exec'd — so the job is that program from here.
//
// Not while the body holds a trap of any kind: a fork that has to answer one
// cannot have become a program that does not know it. Measured beside the
// rows above — `{ trap 'print U' USR1; sleep } &` killed with TERM leaves the
// `sleep` running, and `{ trap - TERM; sleep } &` does not.
//
// An EXIT trap counts, and it is the one the copy of the traps does not hold,
// so the caller says: `{ trap 'print X' EXIT; sleep } &` killed with TERM
// leaves the `sleep` running and prints no X.
func (b *bodyInbox) reachedATail(c *syntax.SimpleCmd, exitTrap bool) {
	if c == nil || !b.tails[c] || exitTrap {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.traps) == 0 {
		b.execd = true
	}
}

// isTheProgram reports whether the job has become the program it exec'd.
func (b *bodyInbox) isTheProgram() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.execd
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
	if sig == 0 {
		return false
	}
	j := named
	if j == nil && len(operand) == 1 {
		j = r.runningJobWithPid(operand[0].pid)
	}
	if j == nil || j.inbox == nil || j.Finished() || j.inbox.isTheProgram() {
		return false
	}
	if !catchableSignal(sig) || !j.inbox.deliver(trapKey(name, sig)) {
		// Left at its default — or one no trap can answer. Where that
		// default ends a process it ends the body, and the body's programs
		// are not sent anything.
		if !endsABodyByDefault(name) {
			return false
		}
		j.inbox.kill(name, sig)
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

// abandonOnDeath ends a background job the moment a signal that ends its
// body arrives, without waiting for the body's goroutine to reach a moment
// between commands. Measured 2026-10-02 on zsh 5.9.2 under `-f -c`:
//
//	{ /bin/sleep 1.37 | cat } & sleep 0.3; kill -TERM $!; wait; print end
//	        `end` at 0.3s, and the `sleep` keeps running
//	{ ( trap 'print S' TERM; sleep 0.6 ); print after $? } & sleep 0.2
//	kill -TERM $!; wait; print end
//	        `end` at 0.2s, and no `S` and no `after`
//
// where this shell waited out the pipeline and the parentheses first. The
// job's status is the signal's; HUP is left to the body, which a dialect may
// read as an orderly exit, and a body that has become its program got no
// death at all.
func (r *Runner) abandonOnDeath(j *Job, ended func(int, syscall.Signal)) {
	b := j.inbox
	if b == nil {
		return
	}
	go func() {
		select {
		case <-b.died:
		case <-j.done:
			return
		}
		name, sig := b.death()
		if name == "HUP" || sig == 0 {
			return
		}
		ended(128+int(sig), sig)
	}()
}
