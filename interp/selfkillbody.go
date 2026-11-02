// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"slices"
	"syscall"
)

// A signal a script aims at **the pid the running shell answers as its own**
// is delivered to that shell's traps.
//
// That sentence has one noun in it and the noun is the *pid*, not the context.
// sendSignal used to key the same rule on `os.Getpid()`, which is the process,
// and the process is the shell only at the top level. Inside a body a real
// shell would have forked, this shell answers "which process am I" with the
// group it leads rather than with the process it is in — see procanchor.go and
// dialect/zsh's subshellPid — so a subshell that killed the number it had just
// been given aimed a real signal at the group's placeholder leader, which is
// not a shell, has no traps, and dies without anything noticing (#4596).
//
// Measured 2026-09-26 against zsh 5.9.2 and bash 5.3.20, both `/opt/homebrew`,
// both `go version -m` → not a Go executable:
//
//	                                          zsh 5.9.2   bash 5.3   here, before
//	( trap 'print T; exit 19' TERM
//	  kill <own pid> )                        T, 19       T, 19      ret=0, 0
//	( trap 'exit 11' USR1
//	  kill -USR1 <own pid> )                  11          11         0
//	( kill <own pid> )        — untrapped     143         143        0
//	( trap 'print E' EXIT
//	  kill <own pid> )        — untrapped     143, no E   143, no E  0, E
//	( kill -KILL <own pid> )                  137         137        0
//	( trap 'print T' TERM
//	  kill $$ )               — the parent    parent dies, sender runs on: unchanged
//
// The last row is the pair that holds the noun fixed. Same construct, same
// context, same signal, same trap — only the *number* differs, and both real
// shells move: the body's own number runs the body's trap, the parent's number
// kills the parent and the body carries on. A rule keyed on "am I in a
// subshell" cannot tell those two apart, and one keyed on `os.Getpid()` gets
// the second right and the first wrong, which is exactly what was shipped.
//
// **This is not a dialect split.** The issue reported that real bash does not
// fire the trap on `( trap … TERM; kill $BASHPID )`; measured here it does,
// both as a script file and as the last command of a `-c`, with the same
// status and the same untrapped 143. So the two shells that can reach the
// shape at all agree about it, there is nothing to ask a Semantics axis, and
// the fix is the core's. `$BASHPID` and `$sysparams[pid]` are the only two
// ways in — both answer through [Runner.SubshellProcessGroup] — so nothing in
// the three dialects without such a parameter can reach this code.
//
// Nothing leaves the process, which is the rule `interp` already holds for
// `$$` and holds for the same reason: a line of script must not be able to
// fire an embedder's handlers. The anchor is *not* signaled either, and that
// is the other half of the fix rather than an omission — it is a placeholder
// whose only job is to exist, and a script that has just been handed the group
// id may still be about to start something in it.

// aimedAtThisBody reports that a pid names the forked body this runner *is*,
// rather than the process it runs in or anyone else's.
//
// The group is read without starting one, and that is not an optimisation: a
// body that has never been asked which process it is has no anchor, and a
// script that was never given the number cannot have named it. Starting one
// here to compare against would fork a process for every `kill` in every
// subshell in order to learn that the answer is no.
func (r *Runner) aimedAtThisBody(pid int) bool {
	// This body's own and never the one around it. `existing` walks outward
	// so that a command a body starts joins the group it is contained in;
	// identity does not walk, because a real shell forked again for the inner
	// body and the inner body is not the outer one. A body around this one
	// is answered too, but by that body and not here: see bodyAroundNamed.
	return r.bodyAnchor.leads(pid)
}

// leads reports that pid is the group this anchor already leads.
func (a *procAnchor) leads(pid int) bool {
	if a == nil || pid <= 0 {
		return false
	}
	a.mu.Lock()
	pgid := a.pgid
	a.mu.Unlock()
	return pgid > 0 && pgid == pid
}

// signalThisBody settles a signal the running forked body aimed at itself.
//
// The shape of the switch is sendSignal's, deliberately: it is the same
// question asked of a shell that has a group instead of a process, so the
// answers line up case for case and a change to one is visible as a difference
// from the other. What differs is which table decides and which shell ends.
//
//   - The table is this body's own. A subshell starts with the parent's
//     handled signals back at their defaults, so the parent's trap is exactly
//     what must not answer here — see trapsubshell.go.
//   - The death is this body's own. signalDeath on the body sets its status to
//     128+N and stops it, and the parent reads that status out of the
//     parentheses as a real shell reads it out of a fork. It does not reach
//     recordSharedDeath, which is where a signal aimed at `$$` goes and which
//     ends the process.
//
// The arrival goes on the body's own pending list rather than the shared one,
// for the reason writeSignalAbsorbed puts a broken pipe there: the process
// never had this signal, and the handler that answers it is the one the body
// set. `kill` drains that list as it returns, as it drains the shell's (see
// biKill) — so the handler runs before the `kill` line's own status is read,
// which is the measured order in both references, and before a body whose
// last command the `kill` was has ended (#7345).
func (r *Runner) signalThisBody(name string, sig syscall.Signal) error {
	key := trapKey(name, sig)
	body, trapped := r.trapTable()[key]
	if !catchableSignal(sig) {
		// KILL and STOP leave a listing rather than a handler, exactly as at
		// the top level: `( trap 'print T' TERM; kill -KILL <own pid> )` is
		// 137 and prints nothing in both references. See sendSignal.
		trapped = false
	}
	switch {
	case trapped && body != "":
		// The same question the top level asks, for the same reason: what a
		// forked body aims at itself is a signal, and the condition a `CHLD`
		// handler answers is a child of its own. See
		// Runner.selfAimedSignalIsTheCondition.
		if !r.selfAimedSignalIsTheCondition(key) {
			break
		}
		if r.signalIsHeldUntilInputOrAChild(key) {
			// Held for the body's own next moment rather than run between
			// its commands — see Runner.signalIsHeldUntilInputOrAChild. A
			// body that ends without waiting for a child runs nothing.
			r.selfHeldForInput = append(r.selfHeldForInput, key)
			break
		}
		r.selfSignaledBody(key)
	case !trapped && fatalSignal(name, sig):
		// **Without asking whether the shell ignores it untrapped**, which
		// is the one place this switch parts from sendSignal's and is
		// measured rather than reasoned. A shell that is not interactive
		// ignores an untrapped QUIT in three of the panel's columns —
		// Semantics.QuitIgnoredWhenNotInteractive, and `kill -QUIT $$` is 0
		// and silent there — and a body a real shell would have *forked*
		// takes the signal all the same. Measured 2026-09-26 against zsh
		// 5.9.2 and bash 5.3.20, `( kill -QUIT <own pid> ); echo $?`: 131 in
		// both, with bash's located notice beside it, where the top level of
		// the same run is 0 (#4724).
		//
		// With `-i` as well as without, which is what makes it unconditional
		// rather than a second reading of the mode: both references answer
		// 131 for the body in either mode, so the shell's own disposition —
		// inherited ignore or decision taken each time — is not what the
		// body is asking. The fork starts from the default.
		r.signalDeath(name, sig)
	default:
		// An ignored signal, and the ones whose default action suspends or
		// resumes rather than ends. sendSignal hands a stop to
		// Runner.StopThisProcess because the process really can be stopped;
		// a forked body cannot, since it is a goroutine, and the only process
		// this pid names is a placeholder that is not running the script.
		// Stopping *that* would suspend nothing the script can see and would
		// take the group's leader away from a body still using it, so
		// nothing is sent — as for the rest of this case at the top level.
	}
	return nil
}

// selfSignaledBody records a signal the running body aimed at itself.
//
// selfSignaled's argument, one boundary in: recorded rather than sent, so
// delivery is deterministic by construction instead of by whether os/signal's
// goroutine got to run — and on the body's list, because the process never had
// the signal and the parent must not run its handler for it.
func (r *Runner) selfSignaledBody(key string) {
	r.selfPending = append(r.selfPending, key)
	s := r.sigs()
	s.mu.Lock()
	defer s.mu.Unlock()
	// A `wait` blocked elsewhere re-reads what is pending when it is poked,
	// and the poke is cheap enough not to be worth deciding about.
	s.poke()
}

// A copy of the body — a pipeline element, a nested `( … )`, anything a real
// shell would have forked again from it — that aims a signal at the body's
// number is aiming at **the body**, and it is the body that answers.
//
// The copy has an anchor of its own, so aimedAtThisBody says no to it, and
// until #7233 the number then went to kill(2) as somebody else's process. It
// is not: it is the body's placeholder, which has no traps and dies of USR1
// or TERM. Nothing ran the body's handler, and the group the body's commands
// join went with the placeholder — a pipeline's next element, checked into
// that group a moment before the kill and started a moment after, failed with
// `fork/exec /bin/cat: operation not permitted`.
//
// Measured 2026-11-01 under `-c` against bash 5.3.20 (`$BASHPID`) and zsh
// 5.9.2 (`$sysparams[pid]` after `zmodload zsh/system`), which agree in every
// row; ksh93u+ 2012 has no such parameter and cannot reach the shape:
//
//	( trap 'echo U' USR1; p=<pid>; { kill -USR1 $p; } | /bin/cat; echo after )
//	        U after          here, before: cat not permitted, after
//	( trap 'echo U' USR1; p=<pid>; { kill -USR1 $p; echo l; } | /bin/cat; echo after )
//	        l U after        the handler runs once the copy is done
//	( trap 'echo T' TERM; p=<pid>; (kill $p; echo inner); echo after )
//	        inner T after    here, before: inner after
//	( trap 'echo U' USR1; p=<pid>; (kill -USR1 $p) )
//	        U                the body's end is a moment between commands too
//	( p=<pid>; { kill -USR1 $p; } | /bin/cat; echo after )
//	        128+N, no after  the body dies of it; here, before: after, 0
//	( p=<pid>; (kill -USR1 $p; echo inner); echo after )
//	        inner, 128+N     the copy runs on; here, before: inner after, 0
//	( trap '' USR1; p=<pid>; { kill -USR1 $p; } | /bin/cat; echo after )
//	        after            here, before: cat not permitted, after
//
// So every row is the body's own `kill $BASHPID`, answered once the copy has
// finished: the copy posts the signal on the body's process, and the body
// settles it through signalThisBody at its next moment between commands —
// or, where the moment is one every shell agrees on, as its wait for the copy
// ends (answerCopiesAsTheWaitEnds) — the same switch, on the
// goroutine that owns the table it reads. Not a dialect split, so nothing
// asks a Semantics axis.
//
// Only while the body is running. Once its process has ended the number names
// nothing a copy can reach, and the send is what it always was.

// bodyArrival is one signal a copy of a body aimed at it.
type bodyArrival struct {
	name string
	sig  syscall.Signal
}

// bodiesAround yields each body around this runner that is still running,
// innermost first: the body this one was copied from, then the one that was
// copied from, out to the shell, which is not one of them. A body that has
// ended is passed over and the walk goes on past it, since a job a body
// backgrounded outlives the body and not necessarily the bodies around it.
//
// The one walk outward, for both of the questions asked of it: which body a
// number names (bodyAroundNamed), and which bodies a signal to the group they
// share reaches (signalTheGroupFromASubshell).
func (r *Runner) bodiesAround(yield func(*Process) bool) {
	if r.process == nil {
		return
	}
	for p := r.process.outer; p != nil; p = p.outer {
		if !p.ended() && !yield(p) {
			return
		}
	}
}

// bodyAroundNamed is the running body around this runner whose group pid is,
// if there is one: the body this one was copied from, or one further out.
func (r *Runner) bodyAroundNamed(pid int) *Process {
	for p := range r.bodiesAround {
		if p.anchor.leads(pid) {
			return p
		}
	}
	return nil
}

// signalTheBodyAround leaves a signal for a body around this runner. Locked,
// because the copy and the body are different goroutines, and a job the body
// backgrounded runs beside it rather than before it.
func (r *Runner) signalTheBodyAround(p *Process, name string, sig syscall.Signal) {
	p.mu.Lock()
	p.arrivals = append(p.arrivals, bodyArrival{name: name, sig: sig})
	p.posted.Store(true)
	p.mu.Unlock()
	s := r.sigs()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.poke()
}

// copiesSignaled reports that a copy of this body has left it something.
func (r *Runner) copiesSignaled() bool {
	return r.process != nil && r.process.posted.Load()
}

// settleCopiesSignals answers what copies of this body aimed at it, as its
// own `kill` would have been answered, and reports whether one ended it.
func (r *Runner) settleCopiesSignals() bool {
	if !r.copiesSignaled() {
		return false
	}
	p := r.process
	p.mu.Lock()
	got := p.arrivals
	p.arrivals = nil
	p.posted.Store(false)
	p.mu.Unlock()
	for _, s := range got {
		was := r.ctl
		_ = r.signalThisBody(s.name, s.sig)
		if r.ctl != was {
			// It died of one, and a process killed by a signal runs
			// nothing more: not the next handler, and not this one's.
			return true
		}
	}
	return false
}

// A copy that is an element of a pipeline the body runs is answered **as the
// body's wait for the pipeline ends**, and not at the body's next command.
//
// Waiting for its next command put the handler after the rest of an and-or
// list, outside a redirection the pipeline was inside, and nowhere at all
// where the pipeline was the last thing a simple-command element ran (#7354).
// Measured 2026-11-02 under `-c`, each shell in a process group of its own,
// the body's pid from `$BASHPID`, `$sysparams[pid]` or a child's `$PPID`:
//
//	( trap "echo U" USR1; p=<pid>; { kill -USR1 $p; } | /bin/cat && echo a )
//	        U, a         bash 5.3.20, bash 3.2.57, zsh 5.9.2, dash;
//	                     here, before: a, U
//	( trap "echo U >&2" USR1; p=<pid>
//	  eval '{ kill -USR1 $p; } | /bin/cat' 2>/dev/null; echo b )
//	        b            the same four: inside the `eval`'s redirection;
//	                     here, before: U, b
//	… { { kill -USR1 $p; } | /bin/cat; } 2>/dev/null; echo b
//	        b            the same four, inside a group's too
//	eval 'trap "echo U" USR1; p=<pid>; { kill -USR1 $p; } | /bin/cat' | /bin/cat
//	        U            all five, ksh93u+ included; here, before: nothing
//	( trap 'echo U $?' USR1; p=<pid>; false
//	  { kill -USR1 $p; } | (/bin/cat; exit 3); echo st=$? )
//	        U 3, st=3    the same four, zsh too: the handler sees the
//	                     pipeline's status, not the one from before it
//
// ksh93u+ runs a `( … )` in its own process, so it reaches only the element
// row. Not a dialect split, so nothing asks a Semantics axis.
//
// **A lone `( … )` and a `$( … )` copy are answered by the same moves**, at
// the moments every shell agrees on: before an and-or list goes on, as the
// simple command that held a `$( … )` ends, and as a body that is one simple
// command ends (#7358). Waiting for the next statement put the handler after
// the rest of the list, outside the `eval` a `$( … )` was in, and nowhere at
// all at a simple-command element's end. Measured 2026-11-02 the same way:
//
//	( trap "echo U" USR1; p=<pid>; ( kill -USR1 $p ) && echo a )
//	        U, a         bash 5.3.20, bash 3.2.57, zsh 5.9.2, dash;
//	                     here, before: a, U
//	( trap 'echo U $?' USR1; p=<pid>; false
//	  ( kill -USR1 $p; exit 3 ) || echo a$? )
//	        U 3, a3      the same four, zsh too: the copy's status
//	( trap "echo U" USR1; p=<pid>; x=$(kill -USR1 $p; echo s) && echo a$x )
//	        U, as        the same four; here, before: as, U
//	( trap "echo U >&2" USR1; p=<pid>
//	  eval 'x=$(kill -USR1 $p)' 2>/dev/null; echo b )
//	        b            the same four and ksh93u+: inside the `eval`'s
//	                     redirection; here, before: U, b
//	eval 'trap "echo U" USR1; p=<pid>; ( kill -USR1 $p )' | /bin/cat
//	        U            the same four, with `x=$(kill -USR1 $p)` too, and
//	                     as a `&` job's body; here, before: nothing
//	( trap 'echo U $?' USR1; p=<pid>; false
//	  x=$(kill -USR1 $p; exit 3); echo st=$? )
//	        U 3, st=3    bash 5.3, 3.2 and dash; zsh U 1, the status from
//	                     before the command, which is what its handler
//	                     between commands sees too
//
// And at the top level, for what a copy aims at `$$`, which is the shell:
// `trap "echo U" USR1; ( kill -USR1 $$ ) && echo a` and the same with `{ kill
// -USR1 $$; } | /bin/cat` are U, a in all four and ksh93u+, where this wrote
// a, U; `( kill $$ ) && echo a` dies of TERM before `a` in all five, where
// this wrote `a` first.
//
// **Not as the wait for a lone `( … )` ends**, which is where zsh answers it:
// with the copy last inside a redirection the shells split, so the moment a
// `( … )` copy's handler runs between there and the next command is left
// where it was. Measured the same way, U written to the redirected stream:
//
//	… { ( kill -USR1 $p ); } 2>/dev/null; echo b
//	        U, b         bash 5.3, 3.2 and dash; b in zsh and here
//	… eval '( kill -USR1 $p )' 2>/dev/null; echo b
//	        U, b         bash 5.3, 3.2 and here; b in zsh and dash
//	… { ( kill -USR1 $p ); } 2>/dev/null       — the body's last command
//	        U            bash 5.3, 3.2 and dash; nothing in zsh and here
//
// zsh runs the handler as the wait ends, inside both redirections; bash at
// the next command, outside both; dash outside the group's and inside the
// `eval`'s, while a function call's redirection holds it in all four. That is
// a split keyed on which construct's redirection ends with the copy, and it
// is #7358's to settle. A `$( … )` copy's handler runs as the substitution
// ends in zsh — `echo x$(kill -USR1 $p; echo s)` is U, xs there and xs, U in
// bash and dash — which is Semantics.HandlersRunAsASubstitutionEnds's shape,
// and is not asked here either: this runs it as the command ends.
//
// The arrival is the one the copy left and it settles through
// settleCopiesSignals, as at the next command; only the moment moves.

// answerCopiesAsTheWaitEnds runs, as the wait for a copy of this shell ends,
// the handlers for what copies aimed at it, and dies of what they aimed at it
// untrapped: a pipeline's end, an and-or list's step, and the end of a simple
// command whose `$( … )` the copy was. seen is the `$?` the handler is shown
// in the dialect whose handler between commands sees an earlier one: the
// pipeline's or the step's status, and for a simple command the status from
// before it.
//
// A body answers what its copies posted and anything else it raised on
// itself (Runner.selfPending). The shell at the top answers only where a copy
// aimed at `$$` — a fatal signal it sent, or a trapped one it raised — so a
// signal from another process waits for the next statement here; at an
// and-or list's step it does not (answerArrivalsBeforeTheListGoesOn).
func (r *Runner) answerCopiesAsTheWaitEnds(ctx context.Context, seen int) {
	if r.ctl != controlNone || !r.aCopysArrivalWaits() {
		return
	}
	r.runPendingTrapsShowing(ctx, seen)
}

// runPendingTrapsShowing runs the handlers waiting, as runPendingTraps does,
// with seen as the status from before the command in the dialect whose
// handler is shown that one.
func (r *Runner) runPendingTrapsShowing(ctx context.Context, seen int) {
	before := r.statusBefore
	r.statusBefore = seen
	r.runPendingTraps(ctx)
	r.statusBefore = before
}

// aCopysArrivalWaits reports whether a copy of this shell has left it
// something to answer.
func (r *Runner) aCopysArrivalWaits() bool {
	if r.inSubshell {
		return r.copiesSignaled() || len(r.selfPending) > 0
	}
	s := r.signals
	return s != nil && !s.nothingArrived() && s.aCopysArrivalWaits()
}

// aCopysArrivalWaits reports whether an arrival waiting for the shell came
// from a copy of it: a death recordSharedDeath noted, or a signal a copy's
// `kill` raised on `$$` — a `( … )`'s or a pipeline element's, which
// selfSignaled records as originElsewhere, or a command substitution's own,
// originSubstitution. Only the shell's own `kill $$` is left out, and it
// answers itself as the builtin returns.
//
// A substitution's own `kill $$` is a copy's like the others, and is answered
// as the simple command that held it ends (#7361). Waiting for the next
// statement ran the handler outside a group's or an `eval`'s redirection the
// command was in. Measured 2026-11-02 under `-c`, each shell in a process
// group of its own, against bash 5.3.20, bash 3.2.57, zsh 5.9.2, ksh93u+
// 2012-08-01 and dash:
//
//	trap "echo U >&2" USR1; { x=$(kill -USR1 $$); } 2>/dev/null; echo b
//	        b            all five; here, before: U, b
//	trap "echo U" USR1; eval 'x=$(kill -USR1 $$)' >/dev/null; echo b
//	        b            all five; here, before: U, b
//	… { x=`kill -USR1 $$`; } 2>/dev/null; echo b    the same: backquotes
//	trap 'echo U $?' USR1; false; x=$(kill -USR1 $$; exit 3) || echo a$?
//	        U 3, a3      bash 5.3, 3.2 and dash; zsh U 1, the status from
//	                     before the command; here, before: U 3 in zsh too
//	trap "echo U" USR1; x=$(kill -USR1 $$) >/dev/null; echo b
//	        U, b         bash 5.3, 3.2, ksh93 and dash: the command's own
//	                     redirection is over by then; zsh b (#7224)
//
// So it is not a split, and nothing asks a Semantics axis: the five agree
// wherever the moment is the command's end, ksh93 whose `$( … )` runs in the
// shell's own process included. What zsh does with it sooner — as it arrives,
// while the substitution still runs — is #7224's and is not modeled here, and
// neither is ksh93 losing it to a second substitution (#7225).
// Semantics.HandlersRunAsASubstitutionEnds is the question for a signal from
// elsewhere during a substitution, and is asked as it was.
func (s *signalState) aCopysArrivalWaits() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.diedSig != 0 {
		return true
	}
	for _, name := range s.pending {
		if slices.ContainsFunc(s.origins[name], raisedByACopy) {
			return true
		}
	}
	return false
}

// raisedByACopy reports that an arrival was raised on `$$` by a copy of the
// shell rather than by the shell's own `kill`.
func raisedByACopy(o trapOrigin) bool {
	return o == originElsewhere || o == originSubstitution
}

// A **group** kill aimed at that same number — `kill -- -$BASHPID`, `kill
// -TERM -$sysparams[pid]`, `kill -0 -$p`, from the body itself or from a copy
// of it — is refused as a group that is not there, where the body was forked
// with the monitor off.
//
// The number is a group here because a placeholder leads one (procanchor.go).
// A shell that forks puts a child in a group of its own only for job control,
// so in the references the body leads nothing and there is no group of that
// number to signal. Measured 2026-11-01 under `-c` against bash 5.3.20
// (`$BASHPID`) and zsh 5.9.2 (`$sysparams[pid]` after `zmodload
// zsh/system`), which agree in every row they can both reach; ksh93u+ 2012 and
// dash have no such parameter and cannot reach the shape:
//
//	( p=<pid>; { kill -- -$p; echo k=$?; } | /bin/cat; /bin/echo after )
//	        no such process, k=1, after
//	        here, before: `/bin/cat: … operation not permitted`, after —
//	        k=0 went to the cat that never started
//	( p=<pid>; kill -- -$p; echo k=$? )           no such process, k=1
//	        here, before: k=0
//	( p=<pid>; (kill -- -$p; echo k=$?) )         the same
//	… kill -TERM -$p, kill -s USR1 -- -$p, kill -0 -$p   the same
//	cat <( p=<pid>; kill -0 -$p; echo k=$? )      the same
//	( set -m; p=$BASHPID; kill -0 -- -$p )        the same — bash
//	set -m; ( set +m; p=$BASHPID; kill -0 -- -$p )        k=0 — bash
//
// The last pair is the noun. Whether the group is there is settled when the
// body is forked, by the monitor of the shell that forks it, and not by the
// monitor at the `kill`: turned on inside the body it makes no group, and
// turned off inside a body forked under it leaves the group standing. So the
// body's process records the monitor at the fork (Process.monitored), and a
// body forked under the monitor keeps its group — in interactive zsh, on a
// terminal, both `( … )` and `<(…)` answer `kill -0 -$sysparams[pid]` with 0,
// which is the shape the scripts procanchor.go was written for spend the
// number on. Who a signal to that group reaches is jobgroupsignal.go's
// question and not this rule's (#7246).
//
// The refusal is ESRCH with nothing sent and the gate not asked, for the
// reason aimsAJobSpecAtItsGroup gives the same answer: the target is absent by
// construction, and a kill(2) made to find that out would reach the
// placeholder. Signal 0 included, since `kill -0 -$p` is the same refusal in
// both references. Not a dialect split, so nothing asks a Semantics axis. The
// group is this body's or one it was copied from, found out through the
// bodies' processes and the anchor each one has, and whether that body is
// still running does not matter: it never had a group to signal.

//
// Under the monitor the body the shell itself forked leads a group only where
// a real shell made it the leader of one: a pipeline element other than the
// first never does, its group being the first element's, and a pipeline's
// first element and a `<(…)` do where the dialect gives them a group at all.
// That one is a split and asks its axis, and only here, at a kill aimed at
// the body's own number (#7318, #7306). See jobgroupsignal.go.
//
// A `$( … )` is not a split and asks nothing: no shell forks it as a job, so
// it leads no group with the monitor or without it, forked by the shell itself
// or inside another body. Measured 2026-11-01, bash 5.3.20 and 3.2.57 and dash
// with the number read back from a child (`p=$(exec /bin/sh -c 'echo
// $PPID')`), bash 5.3.20 also with `$BASHPID`, zsh 5.9.2 with
// `$sysparams[pid]`, bash with no terminal and each shell in a group of its
// own, zsh and dash on a pseudo-terminal:
//
//	set -m; echo $( p=<pid>; kill -0 -$p; echo k=$? )
//	        no such process, k=1 in bash, zsh and dash; here, before: k=0
//	…without `set -m`                             the same, here too
//	set -m; ( p=<pid>; kill -0 -$p; echo k=$? )   k=0: a job's group
//	set -m; ( echo $( p=<pid>; kill -0 -$p; echo k=$? ) )
//	        bash, zsh: no such process; here, before: k=0
//	set -m; ( set -m; echo $( p=<pid>; kill -0 -$p; echo k=$? ) )
//	        bash: no such process, where `( set -m; ( … ) )` is k=0
//
// The last pair is the noun: with job control back on inside the parentheses
// bash makes a `( … )` a group again and a `$( … )` still none, so it is the
// kind of body that decides and not the depth. ksh93u+ forks neither, and the
// number read back from a child is the shell's own (#7324).
//
// And "with the monitor off" means **the monitor of the shell that forks the
// body**, which a forked body does not have: it keeps the option and runs no
// job control under it until it says `set -m` itself. Measured 2026-11-01,
// bash 5.3.20 and 3.2.57 with no terminal, zsh 5.9.2 on a pseudo-terminal:
//
//	set -m; ( ( p=<pid>; kill -0 -$p; echo k=$? ); : )   no such process;
//	        here, before: k=0
//	set -m; ( set -m; ( … ); : )                         k=0
//	set -m; ( set -m; ( ( … ); : ); : )                  no such process
//
// So Process.monitored is set from Runner.monitorInherited at the fork, and a
// `( … )` that is a `&` job's or a pipeline element's whole body is that fork
// and not one inside it (#7329). dash turns job control back on in no forked
// body, `set -m` or not, which is #7331's split.

// aimedAtAGroupNoForkLeads reports that pid names, as a process group, the
// group this body or one it was copied from leads here and would not lead in
// a shell that forks: a body forked by a shell running no job control, a `$(
// … )`, or the one the shell forked under the monitor as something that leads
// no group.
func (r *Runner) aimedAtAGroupNoForkLeads(pid int) bool {
	if pid >= allProcesses {
		return false
	}
	for p := r.process; p != nil; p = p.outer {
		if p.anchor.leads(-pid) {
			if !p.monitored || p.job == aCommandSubstitution {
				return true
			}
			return p.outer == nil && p.job != notAJob && !r.jobLeadsItsGroup(p)
		}
	}
	return false
}
