// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// The command the shell is running, kept because one dialect shows it back to
// the script as a parameter.
//
// It is the same set of places the DEBUG trap fires at, which is not a
// coincidence and is not an assumption either — it was measured, because the
// two could have parted anywhere. With `true` run first so the record holds
// something else, and the value read from inside a construct's own head so
// that no command of a body has overwritten it yet:
//
//	construct                     head reads      so the head recorded
//	for w in "$COMMAND"           `true`          no
//	case "$COMMAND" in            itself          yes
//	[[ "$COMMAND" == true ]]      itself          yes
//	(( n = ${#COMMAND} ))         itself          yes
//	select w in "$COMMAND"        itself          yes
//
// `if`, `while`, `{ }`, `( )` and a function definition fire no head in that
// dialect's reading and record nothing either. The list `for` is the one that
// looks like an exception and is not: its head *is* recorded, once per pass
// and after the word list has been expanded, which is exactly where its DEBUG
// firing is — so the expansion of its own list still reads the command before
// it.
//
// Two rules beyond the sites, both measured on bash 5.3.15 on 2026-09-14:
//
//   - It is recorded whether a trap is set or not. A script reading the
//     parameter with no DEBUG trap anywhere reads the command doing the
//     reading, because the record is written before the command's own words
//     are expanded.
//   - A trap body records nothing. The commands of an action — and the
//     commands of a function the action calls — leave the record alone, which
//     is what lets a DEBUG action name the command it fired for after running
//     commands of its own, and what lets an EXIT or a signal action name the
//     command the script had reached.

// CommandPart is which part of a command is running, for a command whose
// parts run on their own schedule rather than with the command.
type CommandPart int

const (
	// WholeCommand is the command itself. For a compound command that is its
	// head, since the commands of a body are commands in their own right and
	// record themselves.
	WholeCommand CommandPart = iota
	// ArithInit, ArithCond and ArithPost are an arithmetic loop's three
	// expressions. Each is evaluated on its own — the initializer once, the
	// condition before every pass, the step after every body — so each is a
	// thing the shell is running rather than the loop being it.
	ArithInit
	ArithCond
	ArithPost
)

// RunningCommand is the command the shell is running or is about to run.
//
// A node rather than text, because rendering a command back as source is the
// asking dialect's own: the shell with this parameter prints `for w in "$@"`
// where the script wrote `for w`, and puts a space after a `case` head's
// `in`. Nothing here should have to know that.
type RunningCommand struct {
	// Cmd is the command, nil before the shell has started one.
	Cmd syntax.Command
	// Part is which of Cmd's parts is running.
	Part CommandPart
}

// RunningCommand reports the command the shell is running.
//
// For a dialect with a parameter naming it. The zero value means nothing has
// run yet, which is a script's first command reading the parameter before
// anything set it.
func (r *Runner) RunningCommand() RunningCommand { return r.running }

// recordRunning writes the record, unless a trap body is what is running.
func (r *Runner) recordRunning(c syntax.Command, part CommandPart) {
	if r.inTrapBody {
		return
	}
	r.running = RunningCommand{Cmd: c, Part: part}
}
