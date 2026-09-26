// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// A command's assignment prefix whose value could not be expanded.
//
// **The noun is the expansion, not the assignment**, and the two part company
// on a row the panel already answers: a prefix to a *frozen* name is an
// assignment that failed, and bash reports it and runs the command anyway —
// `readonly r=1; r=2 echo RAN` prints `RAN` there. A prefix whose value could
// not be *expanded* costs the command in every column, bash included. So
// Semantics.PrefixRefusalCostsTheCommand is not this question and is never
// asked here; the command is always given up.
//
// Measured 2026-09-26 against bash 5.3.20 (/opt/homebrew/bin/bash), zsh 5.9.2
// (/opt/homebrew/bin/zsh -f), ksh93u+ 2012-08-01 (/bin/ksh) and dash 0.5.12
// (/bin/dash), each snippet on its own line so that giving up the *line* and
// giving up the *shell* can be told apart — on one line the two print the
// same nothing, which is the pairing #1171 was missed for:
//
//	a=$((1/0)) echo RAN; echo SAME      bash  ksh93  zsh    dash
//	                                    ----  -----  ----   ----
//	the command runs                    no    no     no     no
//	`SAME`, the rest of the line         no    yes    no     no
//	the next line                       yes   yes    no     no
//	status the next line reads          1     1      —      —
//
// Three outcomes, and they are the three Runner.prefixRefusalCost already
// produces for a refusal: bash gives up the line, dash ends the shell, and
// zsh and ksh93 do one or the other **by what the command word is**. That
// last split is this file's reason for asking anything at all:
//
//	a=$((1/0)) <cmd>            bash  ksh93  zsh    dash
//	                            ----  -----  ----   ----
//	echo, true, read  regular   line  cmd    shell  shell
//	:, export, eval   special   line  shell  shell  shell
//	f(){...}          function  line  shell  shell  shell
//	/bin/echo         external  line  cmd    cmd    shell
//	command echo                line  cmd    cmd    shell
//	command /bin/echo           line  cmd    cmd    shell
//
// `cmd` there is the third outcome — the command alone is given up, the
// status is the fatal one and the rest of the line still runs — and it is
// catchable: `a=$((1/0)) /bin/echo RAN || echo CAUGHT` prints `CAUGHT` in zsh
// and ksh93 and prints nothing in bash and dash.
//
// That grid is column for column [Semantics.PrefixRefusalFatality]'s, which is
// why no axis is added: zsh holds PrefixRefusalFatalOnACommandThisShellRuns,
// ksh93 PrefixRefusalFatalOnASpecialBuiltinOrFunction, dash and BusyBox ash
// the POSIX preset's PrefixRefusalAlwaysFatal, and bash
// PrefixRefusalNeverFatal. The axis records how far a prefix that **did not
// take** unwinds, and a value that would not expand is as much a prefix that
// did not take as a name that would not accept one. Whether the command still
// runs is the second question, and only a refusal splits the panel on it.
//
// A pipeline is not on that grid and needs nothing from it: every column
// contains the failure in the element, because the element is a child. This
// shell clones a Runner per element, so it is contained here for the same
// reason.
//
// The rest is Runner.failedExpansion's, unchanged and not restated: which
// non-zero status a fatal error carries is FatalErrorStatusIsOne's, and
// whether giving up ends the line or the shell is
// FailedExpansionAbandonsTheLine's. Routing through that one door rather than
// writing the unwinding out again is what keeps a `set -u` refusal and a bad
// subscript — which both arrive here and both have exits of their own — from
// being answered twice.

// prefixWalk is what a command knew before it began working through its
// assignment prefix: whether there is a prefix at all, and whether something
// had *already* failed.
//
// Both halves are load-bearing and the second is the one that was missing. A
// failure is a fact about the whole command and the flags that carry it are
// not the prefix's, so a command with no prefix, or one whose here-document
// body failed before the prefix was reached, must not be given up as though a
// value would not expand: `: <<END` with a division in its body costs the
// command and not the script, which is interp/heredocprocess.go's measurement
// and a different rule from this one.
type prefixWalk struct {
	// written says at least one of the assignments is a prefix rather than a
	// declaration builtin's operand.
	written bool
	// before says a failure was already on the record when the walk began,
	// so nothing this walk did is what failed.
	before bool
}

// beginPrefixWalk takes that snapshot, immediately before the values are
// expanded.
func (r *Runner) beginPrefixWalk(assigns []*syntax.Assign) prefixWalk {
	return prefixWalk{
		written: aPrefixIsWritten(assigns),
		before:  r.expandErr || r.ctl != controlNone,
	}
}

// prefixWalkFailed reports whether a value *this walk* expanded could not be
// expanded.
//
// The loops that apply a prefix read it to stop at the first entry that
// failed, which is unanimous in the panel and was measured with the side
// effect written where it can be seen rather than captured: `a=$((1/0))
// b=$(echo SIDE >&2; echo v) echo RAN` writes no `SIDE` in bash, ksh93, zsh
// or dash. Written `$(echo SIDE)` the probe proves nothing — a substitution's
// output is captured either way, so the entry running and not running print
// the same nothing.
func (r *Runner) prefixWalkFailed(w prefixWalk) bool {
	return w.written && !w.before && (r.expandErr || r.ctl != controlNone)
}

// givesUpForAFailedPrefix ends a command whose assignment prefix could not be
// expanded, and reports whether it did.
//
// Called once per command, after its prefix has been walked and before the
// command is dispatched — the three routes a prefix reaches (a function, a
// builtin, an external) each expand their values in a loop of their own, so
// the call is at the end of each loop and the deciding is in here.
func (r *Runner) givesUpForAFailedPrefix(w prefixWalk, p prefixCommand) bool {
	if !r.prefixWalkFailed(w) {
		return false
	}
	if r.ctl != controlNone {
		// Already unwound by the expansion itself. `${q?word}` is the one
		// that gets here: it ends the *shell* in every column when it is
		// written in an ordinary word, and it is the same sentence and the
		// same fatality here — except in the two columns that would have
		// run this command in a child, where it is contained exactly as a
		// division by zero is. Measured 2026-09-26, `a=${q?bad} <cmd>; echo
		// st=$?; echo AFTER` on three lines: ksh93 carries on at 1 for
		// `echo`, `true`, `/bin/echo` and `command echo` and stops for `:`
		// and a function; zsh 5.9.2 carries on at 1 for `/bin/echo` and
		// `command echo` and stops for the rest; bash 5.3.20 and dash 0.5.12
		// stop for all six. The same six words with `echo ${q?bad}` or
		// `x=${q?bad}` stop in all four, which is what says the containment
		// is the prefix's and not the parameter's.
		//
		// Consulted before r.expandErr for Runner.failedHeading's reason: a
		// failure that has already unwound must not be sent through
		// failedExpansion a second time to have its status rewritten.
		if r.prefixFailureIsThisCommandsAlone(p) {
			r.containTheFailureInTheCommand()
		}
		return true
	}
	if r.prefixFailureIsThisCommandsAlone(p) {
		r.containTheFailureInTheCommand()
		return true
	}
	r.failedExpansion()
	return true
}

// containTheFailureInTheCommand keeps a fatal error inside the command it
// happened in: the status is the fatal one, the command does not run, and the
// shell carries on at the next one.
//
// This is a process boundary reconstructed by hand, which is the standing rule
// for everywhere a real shell relies on one. zsh and ksh93 reach a prefixed
// external — and ksh93 a prefixed regular builtin — by way of a child, so the
// shell that the failure ends is the child rather than the script's own; the
// parent reads a status and goes on. Nothing forks here, so the unwinding is
// taken back instead, and taking it back is what makes the failure
// **catchable**: `a=$((1/0)) /bin/echo RAN || echo CAUGHT` prints `CAUGHT` in
// both columns and prints nothing in bash and dash.
//
// The flags go with the control value. Leaving expandErr set would have the
// next construct to read it — a compound command's heading, which clears them
// on the way in for exactly this reason — abandon itself over a failure that
// has already been paid for.
func (r *Runner) containTheFailureInTheCommand() {
	r.ctl, r.abandon, r.errexitStopped = controlNone, abandonRequested, false
	r.expandErr, r.badSubscript = false, false
	r.setFatalStatus()
}

// prefixFailureIsThisCommandsAlone says the failure costs this command and
// nothing else — no fatal error, and the rest of the line still runs.
//
// The column that gives up the *line* gives it up here too, whatever the
// command word was: bash's rows in the grid above are `line` for all six
// kinds, so it is answered before the kind is looked at and the axis below
// is not asked. What is left is the two columns that end the shell for some
// kinds and not others.
func (r *Runner) prefixFailureIsThisCommandsAlone(p prefixCommand) bool {
	if r.sem().FailedExpansionAbandonsTheLine == Yes {
		return false
	}
	fatal, answered := r.prefixFatalityForTheCommand(p)
	// An unanswered axis falls back to ending the shell, which is what the
	// standard describes, what four of the five columns do for most command
	// words, and what this path already did. See Runner.failedExpansion,
	// where the same reasoning reads the abandon axis rather than asking it:
	// a core run with no dialect has a correct answer to fall back on rather
	// than a missing one to complain about, and it would otherwise be the
	// second complaint on this path in that run.
	return answered && !fatal
}
