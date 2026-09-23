// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// The `CHLD` trap fires once for every child the shell reaps, and this shell
// has no child for most of what a real one forks.
//
// # What bash counts, measured
//
// 2026-09-23, bash 5.3.20 under `env -i PATH=/usr/bin:/bin`, a trap that
// increments a counter and the counter read after a `wait`:
//
//	( exit 0 ) & ×5, one wait          5
//	{ :; } &                           1
//	f &  (a function)                  1
//	a foreground builtin               0
//	a foreground external              1
//	a pipeline of two externals        2
//	x=$(external)                      1
//	( external; external ) &           1
//	( external; external )             1
//
// So it is **once per child the shell forked and reaped**, and nothing else:
// the foreground builtin forks nothing and counts nothing, the pipeline forks
// twice, and a subshell is one child however many commands run inside it. The
// two children a subshell of its own starts are *its* children, not the
// shell's — which the last two rows are the whole evidence for.
//
// # Why the signal could not be the source here
//
// A background job in this shell is a goroutine, so five subshells exiting
// produce no SIGCHLD at all and the trap fired **zero** times where bash fires
// five. The rows that did work were the ones with a real process in them, and
// one of those was working by accident: `( external; external )` in the
// foreground fired **twice** here, because the two processes a subshell starts
// are this *process's* children when the subshell is a goroutine. The shell
// was counting its grandchildren.
//
// Under- and over-counting are the same fact seen twice: what the kernel tells
// this process about is not the set of forks the shell performed. So the
// signal is dropped for this condition — see drainForwarded — and the
// condition is raised where the shell finishes something a real one would have
// forked for. Those places are already enumerated, because a coprocess that
// ended is noticed at exactly the same points: see Runner.retireCoproc.
//
// # Where it is raised, and to whom
//
// A subshell keeps its own arrivals, exactly as a broken pipe does — see
// Runner.brokenPipeAbsorbed, whose two lists this follows. An external
// command inside `( … )` raises on the subshell's list, so the subshell's own
// handler runs and the shell at the top never sees it; the subshell *ending*
// then raises one on the shell, which is the one child bash's parent reaped.
//
// A background job raises on the shared list and never on a subshell's, and
// that is a limit rather than a rule: the raise happens on the job's own
// goroutine, and a subshell's list is written without a lock because only its
// own goroutine touches it. A `( cmd & wait )` therefore has its arrival
// counted by the shell rather than by the parentheses. The alternative is a
// lock on a list that exists to avoid one.
func (r *Runner) childReaped() {
	if !r.trapsChildDeath() {
		return
	}
	if r.inSubshell || r.traps != nil || r.forkedForABackgroundJob {
		// Not the shell at the top, so nothing is recorded: what a subshell
		// reaps are *its* children, and the one arrival the shell gets for
		// the whole of it is raised where the subshell itself is finished
		// with. Recording here as well would count a grandchild, which is
		// what the kernel was doing before this.
		//
		// **A subshell's own handler is not run for its children here, and
		// that is a divergence rather than a rounding.** Measured 2026-09-23:
		// `( trap 'echo S' CHLD; external; external )` prints `S S` in bash
		// 5.3.20 and nothing here, and with an outer trap as well bash prints
		// `S S T` where this prints `T`.
		//
		// What stands in the way is not the counting. A subshell's arrivals
		// have to go on its own list — see Runner.selfPending — and until
		// #4157 that list was read by the wait a `&` job is under: with one
		// recorded, `trap 'echo T' CHLD; ( sleep; echo b ) & wait` returned
		// from the wait before the job had finished and lost the `echo b`.
		// **That wait no longer ends on a child's death** — see
		// Runner.pendingTrap, which is where the reason is written down — so
		// what is left here is a plain gap rather than a blocked one: the
		// shell counts its own children and a subshell counts none.
		return
	}
	r.recordChildDeath()
}

// childReapedByTheShell is childReaped for a raise that happens on another
// goroutine — a background job finishing — where a subshell's own list cannot
// be written safely. See the field comment on Runner.selfPending.
func (r *Runner) childReapedByTheShell() {
	if !r.trapsChildDeath() {
		return
	}
	r.recordChildDeath()
}

// trapsChildDeath reports whether anything would run for a reaped child.
//
// Asked before anything is recorded, because a condition nobody trapped would
// otherwise pile arrivals onto a list nothing drains — and because this is on
// the path of every external command. The table is the subshell's where there
// is one and the process's otherwise, which is where a trap lives: see the
// field comment on Runner.traps.
func (r *Runner) trapsChildDeath() bool {
	if r.traps != nil {
		body, ok := r.traps["CHLD"]
		return ok && body != ""
	}
	s := r.sigs()
	s.mu.Lock()
	defer s.mu.Unlock()
	body, ok := s.traps["CHLD"]
	return ok && body != ""
}

// recordChildDeath puts one arrival on the shared list, to be run between
// commands like any other.
func (r *Runner) recordChildDeath() {
	s := r.sigs()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending = append(s.pending, "CHLD")
	s.poke()
}
