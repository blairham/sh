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

// redirectTargetForItsProcess is redirectTarget with the question of whose
// process expanded the word answered.
//
// The wrapper rather than a flag inside the expander, for the reason
// confineToTheProcess is one: the target expands exactly as it always did,
// and the whole of this is what becomes of the writes and the failure it
// leaves behind.
func (r *Runner) redirectTargetForItsProcess(rd *syntax.Redirect) (string, bool) {
	// The copy is taken only where it could be needed: a target that is one
	// literal — which is most of them, `> out` and `2>&1` alike — expands to
	// itself and cannot write anything, and a command this shell runs itself
	// has nowhere to put the write back from. Saving anyway would clone the
	// whole variable table on every redirection in every script, for a
	// question that is decided before it is asked.
	watching := r.redirForOwnProcess && wordCanWrite(rd.Word)
	var before expansionTables
	if watching {
		before = r.saveExpansionTables()
	}
	name, bad := r.redirectTarget(rd)
	wrote := watching && r.wroteSince(before)
	failed := r.targetExpansionFailed()
	inTheCommand := false
	if r.redirForOwnProcess && (wrote || failed) {
		inTheCommand = r.ask(r.sem().RedirectTargetExpandsInTheCommandsProcess,
			"a redirection target's expansion reaching the shell that ran the command")
		if !inTheCommand && r.unspecified {
			// Nobody answered, so there is no telling whose the write is or
			// whose the failure is. The command does not run: acting on
			// either reading after saying the shells disagree would answer
			// the question anyway.
			r.redirErr = true
			return name, true
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
			r.giveUpTheCommand()
		}
	case failed && r.ctl == controlNone:
		// The expansion happened here, so the failure is this shell's and
		// costs whatever a failed expansion costs it — which is the same
		// door a failed *argument* goes through, axis and all. Reached when
		// the failure reported itself without unwinding; one that has
		// already unwound is carrying its status and is left alone.
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
	return name, bad || failed
}

// targetExpansionFailed reports whether expanding the target left an error
// behind — an unset name under `set -u`, a bad substitution, a division by
// zero — rather than a name.
//
// The two shapes a failed expansion arrives in, which is why this is not one
// field: a fatal one has already unwound and is waiting to be caught, and one
// that only reported itself is still in ordinary flow.
func (r *Runner) targetExpansionFailed() bool {
	return r.pendingFileError() || (r.expandErr && r.ctl == controlNone)
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
