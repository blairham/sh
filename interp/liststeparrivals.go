// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "context"

// What arrived while an and-or list's left side ran is answered before the
// list goes on, wherever it came from (#7362).
//
// A handler runs between commands, and the step from one side of `&&` or `||`
// to the other is one of those moments in every shell: the left side has
// finished and the list has yet to decide what runs next. #7358 answered there
// what a copy of the shell aimed at it; a signal from another process waited
// for the next statement, which put the handler after the rest of the list.
// Measured 2026-11-02 under `-c`, each shell in a process group of its own,
// the left side a real other process — `/bin/kill`, a `perl` that signals
// `$$` and exits 3, or one that waits on a fifo while a background `perl`
// sends the signal and says so:
//
//	trap "echo U" USR1; /bin/kill -USR1 $$ && echo a
//	        U, a          bash 5.3.20, bash 3.2.57, zsh 5.9.2, ksh93u+ and
//	                      dash; here, before: a, U
//	… /bin/kill -USR1 $$ && echo a && echo b
//	        U, a, b       the same five; here, before: a, b, U
//	trap 'echo U $?' USR1; false; <perl, exit 3> $$ || echo a$?
//	        U 3, a3       the same five, zsh too: the left side's status
//	… <the background sender, the left side exiting 3> || echo a$?
//	        U 3, a3       the same five
//	… ! /bin/kill -USR1 $$ || echo a, { /bin/kill -USR1 $$; } && echo a,
//	  /bin/kill -USR1 $$ | /bin/cat && echo a, the list in a function body
//	  or an `eval`, and x=$(/bin/kill -USR1 $$) && echo a
//	        U, a          the same five
//
// The same left side ended by `;`, or as an `if` or `while` condition, was U,
// a here already: the next statement answered it. So did the shell's own
// `kill -USR1 $$`, which answers itself.
//
// The shell at the top answers every arrival here; a body answers only what
// its copies left it and what it raised on itself, as at the other moments a
// wait ends, because a signal aimed at `$$` is the shell's and the body is
// not the shell (Runner.runPendingTraps).
//
// The handler sees the left side's status, the one the list decides on — in
// zsh too, the dialect whose handler between commands is shown an earlier one
// (Semantics.SignalHandlerSeesEarlierStatus). Not a dialect split at this
// moment, so nothing asks a Semantics axis. An arrival from another process
// is answered sooner now, as the left side's pipeline ends — before its `!`
// in zsh, ksh93 and dash, after it in bash — so what this step still finds
// waiting is a copy's, or one that landed after that pipeline's end
// (answerArrivalsAsThePipelineEnds, #7364, #7365).

// answerArrivalsBeforeTheListGoesOn runs, at an and-or list's step, the
// handlers for what arrived while the left side ran, showing seen — the left
// side's status — as `$?` in the dialect whose handler would otherwise see an
// earlier one. A death a copy aimed at the shell ends it here, as at the next
// command.
func (r *Runner) answerArrivalsBeforeTheListGoesOn(ctx context.Context, seen int) {
	if r.inSubshell {
		r.answerCopiesAsTheWaitEnds(ctx, seen)
		return
	}
	if s := r.signals; r.ctl != controlNone || s == nil || s.nothingArrived() {
		return
	}
	r.runPendingTrapsShowing(ctx, seen)
}
