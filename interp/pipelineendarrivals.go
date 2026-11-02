// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"

	"github.com/blairham/sh/internal/sigbook"
	"github.com/blairham/sh/syntax"
)

// What arrived from another process while a pipeline ran is answered as the
// pipeline ends, and the handler sees the status the pipeline left — in zsh
// too, and before a `!` inverts it everywhere but in one case of bash's
// (#7364, #7365).
//
// A real shell waits for the commands it starts, and a signal that lands while
// it waits is answered once the wait is over, before the shell goes on to
// whatever holds the pipeline: an `if` that has yet to decide its status, a
// group whose redirection is still in force, a `!`. This shell waited for the
// next statement, which put the handler after all of those. Measured
// 2026-11-02 under `-c`, each shell in a process group of its own, `S` a
// `/bin/sh` script that signals `$$`, lingers a tenth of a second and exits
// with its second operand, and every row after `trap 'echo U $?' USR1; false;`:
//
//	$S $$ 3; echo b$?
//	        U 3, b3       bash 5.3.20, bash 3.2.57, zsh 5.9.2, ksh93u+ and
//	                      dash; here, before: U 1 in zsh
//	if $S $$ 3; then :; else echo e$?; fi       U 3, e3 in all five
//	while $S $$ 0; do echo w$?; break; done     U 0, w0 in all five
//	f() { $S $$ 3; }; f; echo b$?               U 3, b3 in all five
//	$S $$ 3, the last command                   U 3 in bash, zsh and dash;
//	                                            ksh93 runs it in place of itself
//	/bin/kill -USR1 $$; echo b$?                U 0, b0 in all five
//	        here, before, every one of them U 1 in zsh
//	if $S $$ 3; then :; fi; echo b$?
//	        U 3, b0       all five; here, before: U 0 (zsh U 1), the `if`'s
//	{ $S $$ 3; } >/dev/null; echo a$?
//	        a3            all five: the handler writes into the group's
//	                      redirection; here, before: U 3, a3
//	! { $S $$ 3; }; echo b$?                    U 3, b0 in all five
//	! $S $$ 3 | /bin/cat; echo b$?
//	        U 0, b1       all five: before the `!`; here, before: U 1
//
// zsh's handler sees the status from before the command only where it runs
// inside the shell's own builtin — `false; kill -USR1 $$` and an outside
// arrival that interrupts `wait` are U 1 there, where bash shows `wait`'s 158
// — or as a substitution ends (Semantics.SignalHandlerSeesEarlierStatus). As
// a wait for a child ends, the child's status has landed already, so the
// handler here is shown it: a program, a `( … )`, an element of a pipeline of
// several. A pipeline of builtins alone keeps the earlier status.
//
// The one split is a negated pipeline of one command, and it is
// Semantics.SignalHandlerSeesTheNegatedStatus: bash answers after the `!` and
// its handler sees the inverted status; zsh, ksh93 and dash answer before it.
// A pipeline of several elements is answered before the `!` in all five.
//
//	! $S $$ 3; echo b$?
//	        bash 5.3, 3.2: U 0, b0      zsh, ksh93, dash: U 3, b0
//	! $S $$ 3 && echo a$?                       the same split, a0
//	true; ! $S $$ 0; echo b$?
//	        bash: U 1, b1               zsh, ksh93, dash: U 0, b1
//	! ( $S $$ 3 ); echo b$?                     the same split as the first
//	f() { ! $S $$ 3; }; f; echo b$?             the same split as the first
//
// Only the shell at the top answers here, and only for an arrival from
// another process. A copy's `kill $$` keeps its moments — the next command in
// bash and dash, where `{ (kill -USR1 $$); } >/dev/null` writes U outside the
// group's redirection — and so does the shell's own `kill`, which answers
// itself.

// answerArrivalsAsThePipelineEnds runs, as a pipeline ends, the handlers for
// what arrived from another process while it ran. Where the pipeline waited
// for a child, the dialect whose handler between commands sees an earlier
// status is shown the pipeline's; where it ran builtins only, the earlier one
// still, as zsh shows for an arrival that interrupts its `wait`.
func (r *Runner) answerArrivalsAsThePipelineEnds(ctx context.Context) {
	if !r.anArrivalFromAnotherProcessWaits() {
		return
	}
	seen := r.statusBefore
	if r.waitedForAChild {
		seen = r.status
	}
	r.runPendingTrapsShowing(ctx, seen)
}

// settleWhatAChildSent waits, as a pipeline that waited for a child ends, for
// the runtime to hand this shell whatever signals it has already received
// (#7378).
//
// A signal another process sends reaches this process before the sender
// exits, and the kernel hands it to the runtime's handler then — on whichever
// thread it chose. But the handler does not write to the channel the shell
// reads: a goroutine of os/signal's does, when it is next scheduled, and the
// shell that had just reaped the sender went on to look before that goroutine
// had run. Measured 2026-11-02 over 1000 runs each of `trap 'echo U' USR1;
// /bin/kill -USR1 $$; echo b`, bash 5.3.20, bash 3.2.57, zsh 5.9.2, ksh93u+
// and dash wrote `U b` every time; this shell wrote `b U` or a bare `b` — the
// handler a command late, or never — in 174 of 1000 on Linux at idle and 54
// of 1000 on macOS with every core busy, and in none with a sender that
// lingers. A probe of this wait found the arrival still on its way in 195
// runs of 1000, and those were the late ones.
//
// So "a child this shell waited for has ended" is a barrier: what the
// runtime had received by then is on the channel before the shell looks. It
// costs a lock and a map lookup where no handler is set, and two loads more
// where one is and nothing is on its way; it waits only for an arrival the
// runtime already has, so one sent later is answered where it lands, as
// before.
//
// A signal the kernel has given a thread that has yet to run its handler is
// not the runtime's yet, and nothing here can see it: the kernel picks the
// thread, and a shell of one thread is always the one it picks. Measured over
// the same 1000 runs with this barrier, that is 0 at idle on both platforms and
// 1 or 2 with every core busy.
//
// Only the shell at the top: it is the one whose handlers the process runs.
// And with no window open that ignores signals on the whole process, since
// sigbook.Settle subscribes for a moment to a signal the shell holds.
func (r *Runner) settleWhatAChildSent() {
	if !r.waitedForAChild || r.inSubshell || r.ctl != controlNone || r.signals == nil {
		return
	}
	spawnDispositions.RLock()
	defer spawnDispositions.RUnlock()
	sigbook.Settle(r.signals.ch)
}

// handlerSeesTheNegatedStatus reports whether what arrived from another
// process during a negated pipeline is answered after its `!` rather than
// before. Asked only where it decides something: a pipeline of one command,
// with such an arrival waiting. See Semantics.SignalHandlerSeesTheNegatedStatus.
func (r *Runner) handlerSeesTheNegatedStatus(p *syntax.Pipeline) bool {
	if len(p.Cmds) != 1 || !r.anArrivalFromAnotherProcessWaits() {
		return false
	}
	return r.ask(r.sem().SignalHandlerSeesTheNegatedStatus,
		"the status the handler of a signal that lands during a negated command sees")
}

// anArrivalFromAnotherProcessWaits reports whether the shell at the top has a
// handler waiting for a signal another process sent it.
func (r *Runner) anArrivalFromAnotherProcessWaits() bool {
	if r.inSubshell || r.ctl != controlNone {
		return false
	}
	s := r.signals
	return s != nil && !s.nothingArrived() && s.handlerWaitingFromAnotherProcess()
}

// handlerWaitingFromAnotherProcess reports whether a handler is waiting for an
// arrival that neither the shell's own `kill` nor a copy's raised: more
// arrivals of a condition waiting than the shell recorded raising. A child's
// death is never one, as for handlerWaitingFromElsewhere.
func (s *signalState) handlerWaitingFromAnotherProcess() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.drainForwarded()
	for _, name := range s.pending {
		if name == "CHLD" || s.traps[name] == "" {
			continue
		}
		waiting := 0
		for _, other := range s.pending {
			if other == name {
				waiting++
			}
		}
		if waiting > len(s.origins[name]) {
			return true
		}
	}
	return false
}
