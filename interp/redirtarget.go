// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// A redirection's *target* is expanded where the command it belongs to runs,
// which is the other half of what heredocprocess.go says about a body. This
// file is the target's half.
//
// Measured 2026-09-07 and again 2026-09-10, `env -i PATH=/usr/bin:/bin` with
// a scratch HOME, over a script file, against bash 5.3.15, ksh93u+, zsh 5.9.2
// and dash. Two questions, and they are one property seen twice.
//
// **Where does a write in the target land?** `unset u`, then a target of
// `"${u:=made}"`, and `u` read on the next line:
//
//	                                bash  ksh93  zsh  dash
//	cat /dev/null >T  (external)    gone   gone gone   KEPT
//	cat /dev/null >T | cat          gone   gone gone   gone
//	cat /dev/null >T &              gone   gone gone   gone
//	: >T              (builtin)     KEPT   KEPT KEPT   KEPT
//	exec 3>T          (exec)        KEPT   KEPT KEPT   KEPT
//	f >T              (function)    KEPT   KEPT KEPT   KEPT
//	{ :; } >T         (group)       KEPT   KEPT KEPT   KEPT
//
// **What does a failed expansion cost?** `set -u` and a target of `"$NOPE"`,
// with a line after it to say whether the script is still running:
//
//	                                bash  ksh93  zsh  dash
//	cat /dev/null >T  (external)   alive  alive alive  DEAD
//	cat <T            (external)   alive  alive alive  DEAD
//	cat /dev/null >T | cat         alive  alive alive alive
//	: >T              (builtin)     DEAD   DEAD  DEAD  DEAD
//	exec 3>T          (exec)        DEAD   DEAD  DEAD  DEAD
//
// The line both tables draw is the line heredocprocess.go's draws: whether
// the shell runs the command as a process of its own. Everything the shell
// runs itself keeps the write and dies of the failure, in every column,
// because there is no other process for either to land in — and for a command
// that *is* a process, three columns lose the write and survive the failure
// while dash keeps it and stops.
//
// So one answer covers both consequences, which is right: they are the same
// fact about where the expansion happened. It is a second axis rather than a
// widening of the body's, because dash splits the other way there — a body's
// failed expansion costs dash the command and not the script — and one answer
// cannot say both (#1228).
//
// Every diagnostic in the second table is **one** diagnostic, and that part is
// unanimous: a target whose expansion failed is not opened. Here the empty
// string was opened afterwards, so a script got the unset name and then `:
// No such file or directory`, a complaint about a file nobody wrote. That is
// core, and it holds whoever runs the command.

// redirOwner says whose process a command's redirections are being opened in.
//
// Three values rather than two, because the two consequences this file is
// about — where a write in the target lands, and what a failure in it costs —
// are answered for a **subshell** by a different column split than for a
// command. `> "${u:=made}"`, measured 2026-09-26:
//
//	                            bash  zsh  ksh93  dash  ash
//	cat /dev/null > …  a command gone gone  gone  KEPT  KEPT
//	( : ) > …          a subshell gone gone  KEPT  KEPT  KEPT
//
// ksh93 is the row a boolean cannot carry, so the parentheses are an owner of
// their own with an axis of their own (#4695).
type redirOwner uint8

const (
	// redirOwnerThisShell is a command this shell runs itself: a builtin, a
	// function, a group, a loop. There is no other process, so the write
	// stays where it landed and what is left is whose failure it is — see
	// Runner.targetFailureInThisShell.
	redirOwnerThisShell redirOwner = iota

	// redirOwnerTheCommand is a command that becomes a process of its own,
	// which is what [Semantics.RedirectTargetExpandsInTheCommandsProcess]
	// and [Semantics.HeredocExpandsInTheCommandsProcess] are asked of.
	redirOwnerTheCommand

	// redirOwnerASubshell is a `( … )`. A real shell forks for it, so the
	// question is the same question — and the panel does not give it the
	// same answer, which is why it is a value here rather than the one
	// above. See
	// [Semantics.RedirectTargetOnASubshellExpandsInTheSubshell].
	redirOwnerASubshell
)

// redirectTargetForItsProcess is redirectTarget with the question of whose
// process expanded the word answered.
//
// The wrapper rather than a flag inside the expander, for the reason
// confineToTheProcess is one: the target expands exactly as it always did,
// and the whole of this is what becomes of the writes and the failure it
// leaves behind.
func (r *Runner) redirectTargetForItsProcess(rd *syntax.Redirect) ([]string, bool) {
	// The copy is taken only where it could be needed: a target that is one
	// literal — which is most of them, `> out` and `2>&1` alike — expands to
	// itself and cannot write anything, and a command this shell runs itself
	// has nowhere to put the write back from. Saving anyway would clone the
	// whole variable table on every redirection in every script, for a
	// question that is decided before it is asked.
	watching := r.redirOwner != redirOwnerThisShell && wordCanWrite(rd.Word)
	var before expansionTables
	if watching {
		before = r.saveExpansionTables()
	}
	names, bad := r.redirectTarget(rd)
	wrote := watching && r.wroteSince(before)
	failed := r.targetExpansionFailed()
	inTheCommand := false
	if r.redirOwner != redirOwnerThisShell && (wrote || failed) {
		inTheCommand = r.targetExpandedElsewhere()
		if !inTheCommand && r.unspecified {
			// Nobody answered, so there is no telling whose the write is or
			// whose the failure is. The command does not run: acting on
			// either reading after saying the shells disagree would answer
			// the question anyway.
			r.redirErr = true
			return names, true
		}
	}
	switch {
	case inTheCommand:
		if wrote {
			r.restoreExpansionTables(before)
		}
		if failed {
			// The error happened in a process that is not this shell, so
			// what it costs is the command. Same mechanism as a body's, and
			// deliberately the same one: a second copy of an abandonment
			// boundary is a second place for it to be subtly different.
			r.giveUpTheCommand(redirTargetBoundary)
		}
	case failed && r.redirOwner != redirOwnerTheCommand:
		// The word was expanded here — because the shell runs this command
		// itself and there is no other process, or because the column
		// expands a subshell's target out here even though a real shell has
		// forked — so what is left is whose failure that is. See
		// targetFailureInThisShell.
		r.targetFailureInThisShell()
	case failed && r.ctl == controlNone:
		// The command is a process of its own and this column expands its
		// target here anyway, so the failure is this shell's and costs
		// whatever a failed expansion costs it — the same door a failed
		// *argument* goes through, axis and all. Reached when the failure
		// reported itself without unwinding; one that has already unwound is
		// carrying its status and is left alone.
		r.failedExpansion()
	}
	if failed {
		// Whoever the failure belongs to, the command does not run: its
		// redirections did not come out. Without this the command ran with
		// the stream it was redirecting *away from* — `cat < "$NOPE"` sat
		// reading the terminal — and then reported its own status over the
		// failure's.
		r.redirErr = true
	}
	return names, bad || failed
}

// targetExpandedElsewhere asks whichever axis owns this redirection whether
// the target's word was expanded somewhere other than this shell.
//
// One question, two axes, and the owner picks: a command that becomes a
// process of its own is
// [Semantics.RedirectTargetExpandsInTheCommandsProcess], and a `( … )` is
// [Semantics.RedirectTargetOnASubshellExpandsInTheSubshell]. They are not the
// same axis because they are not the same split — ksh93 answers yes to the
// first and no to the second.
func (r *Runner) targetExpandedElsewhere() bool {
	if r.redirOwner == redirOwnerASubshell {
		return r.ask(r.sem().RedirectTargetOnASubshellExpandsInTheSubshell,
			"a subshell's redirection target expanding in the subshell")
	}
	return r.ask(r.sem().RedirectTargetExpandsInTheCommandsProcess,
		"a redirection target's expansion reaching the shell that ran the command")
}

// targetFailureInThisShell settles what a redirection **target** that would
// not expand costs, on a command this shell runs itself.
//
// The panel splits over whose failure it is, and the split is not the one
// [Semantics.RedirectTargetExpandsInTheCommandsProcess] makes: that axis is
// about a command which *is* a process of its own, and here there is no other
// process for the word to have been expanded in. Measured 2026-09-26 with
// `-c`, the failing redirection on its own line and `echo "after st=$?"` on
// the next, against bash 5.3.20, zsh 5.9.2 under `-f`, ksh93u+ 2012-08-01,
// dash 0.5.12 and BusyBox v1.37.0 in the pinned alpine image. A target of
// `$(( 1/0 ))`:
//
//	                            bash   zsh    ksh93  dash   ash
//	: < $((1/0))     special     st=1   stops  stops  stops  stops
//	read x < …       regular     st=1   stops  st=1   stops  stops
//	f < …            function    st=1   stops  st=1   stops  stops
//	{ :; } < …       group       st=1   stops  st=1   stops  stops
//	command : < …    command     st=1   stops  st=1   stops  stops
//
// ksh93 alone grades it as the **redirection's** failure, which is why the
// only row it stops on is the one POSIX makes fatal for a failed redirection
// — and the noun is a *special builtin*, not "a command the shell runs
// itself": the function, the group and `command :` are all commands it runs
// itself and all three carry on at 1. The other four grade it as **this
// shell's own failed expansion**, which is bash's line and a fatal error in
// the other three, whatever it was written on. See
// [Semantics.RedirectTargetFailureIsTheRedirections] for the pairs that tell
// the two readings apart.
//
// It is a field of its own rather than a second reading of
// [Semantics.HeredocBodyFailureIsTheRedirections] because dash and BusyBox
// ash answer the two differently: `read x <<END` with `$(( 1/0 ))` in the
// body carries on at 2 and at 1 there, where `read x < $(( 1/0 ))` ends both
// shells at 2 (#4689).
//
// The caller has already established the failure, so the two readings are the
// whole of this: the give-up boundary the external half of the same
// redirection already uses — a second abandonment boundary is a second place
// for it to be subtly different — or the door a failed *word* goes through.
// The command does not run either way, which the caller sets.
func (r *Runner) targetFailureInThisShell() {
	if r.ask(r.sem().RedirectTargetFailureIsTheRedirections,
		"whose failure a redirection target that will not expand is") {
		// The redirection's, and that boundary is where the status a failed
		// redirection carries is taken — which is the whole of what
		// separates this from a fatal error in the columns that number the
		// two differently.
		r.giveUpTheCommand(redirTargetBoundary)
		return
	}
	if !r.unspecified && r.ctl == controlNone {
		// This shell's own failed expansion, axis and all. Reached only
		// where the failure reported itself without unwinding; one that has
		// already unwound is carrying its status and its reach and is left
		// exactly alone, which is what makes `${q?word}` in a target end the
		// shell in these columns as it does in an ordinary word.
		r.failedExpansion()
	}
	// Nobody answered leaves both readings alone: the command does not run,
	// which the caller sets, and acting on either reading after saying the
	// shells disagree would answer the question anyway.
}

// targetExpansionFailed reports whether expanding the target left an error
// behind — an unset name under `set -u`, a bad substitution, a division by
// zero — rather than a name.
//
// The three shapes a failed expansion arrives in, which is why this is not
// one field: a fatal one has already unwound and is waiting to be caught, one
// that gave up the statement has unwound a shorter way, and one that only
// reported itself is still in ordinary flow.
//
// The middle shape is the same event as the first seen from the other side of
// Semantics.FailedExpansionAbandonsTheLine — an unmatched pattern in `cat <
// nosuch*` under a `shopt` name that refuses one. Reading only the fatal
// shape let the shell that gives up the statement fall through to opening the
// pattern as a filename, so the complaint was followed by a `No such file or
// directory` about the same word.
func (r *Runner) targetExpansionFailed() bool {
	return r.pendingFileError() || r.ctl == controlAbandon ||
		(r.expandErr && r.ctl == controlNone)
}

// wordCanWrite reports whether expanding this word could write anything —
// which is to say whether any of it is an expansion at all.
//
// A literal cannot: `> out` is the name `out` however many times it is
// expanded. Anything else might, because `${u:=x}`, `$(( n++ ))` and a
// command substitution all reach the variable table, and a word carrying one
// of them is not distinguishable from a word carrying a plain `$v` without
// expanding it.
func wordCanWrite(w *syntax.Word) bool {
	if w == nil {
		return false
	}
	for _, s := range w.Spans {
		if s.Kind != syntax.Literal {
			return true
		}
	}
	return false
}
