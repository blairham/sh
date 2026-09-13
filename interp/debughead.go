// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"strings"

	"github.com/blairham/sh/syntax"
)

// Which commands other than the simple ones fire the DEBUG trap.
//
// Every column that has the condition fires it before a simple command, and
// this engine fired it there and nowhere else — with a comment saying a
// compound heading fires nothing, which is measured to be wrong in all four
// of the panel's columns that have a DEBUG trap at all.
//
// Measured 2026-09-13 with `trap 'echo D $LINENO' DEBUG` over a script of one
// construct per line:
//
//	construct              bash 5.3 / as sh / 3.2 / ksh93   zsh
//	for w in a b           the head on every pass           the head once
//	for w in               nothing, the loop never runs      the head once
//	select                 the head once, before the menu   the head once
//	case                   the head once                    the head once
//	[[ … ]]                the head once                    the head once
//	(( … ))                the head once                    the head once
//	for ((i;c;n))          each of the three, per pass      the head once
//	if / while / until     nothing                          the head once
//	{ … } / ( … )          nothing                          the head once
//	f() { … } (defining)   nothing                          the head once
//	repeat                 no such construct                the head once
//
// So the panel gives two readings rather than a rule with exceptions, and
// which one a dialect takes cannot be derived from anything else it answers.
// dash and ash have no DEBUG condition and never reach the question.

// DebugTrapHeads is which compound commands fire the DEBUG trap.
type DebugTrapHeads int

const (
	// DebugTrapHeadsWordAndArithmetic fires at the heads whose own work is
	// expansion or arithmetic rather than a command: `case`, `[[`, `((` and
	// `select` once each, the list `for` on every pass, and each of the
	// arithmetic `for`'s three expressions every time one is evaluated.
	// Nothing else. bash 5.3, that build invoked as `sh`, bash 3.2 and
	// ksh93, so it is the zero value.
	//
	// `for` firing per pass where `select` fires once is measured and not a
	// slip: a two-reply `select` writes one head and two bodies, and a
	// two-item `for` writes two heads and two bodies.
	DebugTrapHeadsWordAndArithmetic DebugTrapHeads = iota
	// DebugTrapHeadsEveryCompound fires once at the head of every command
	// that is not a simple one, a loop's passes included in that once and a
	// function definition among the heads: zsh.
	DebugTrapHeadsEveryCompound
	// DebugTrapHeadsNone fires at simple commands alone. No column answers
	// it — it is what a dialect with no DEBUG condition would say if it were
	// ever asked, and it is what this engine did before the question was
	// one.
	DebugTrapHeadsNone
)

func (d DebugTrapHeads) String() string {
	switch d {
	case DebugTrapHeadsEveryCompound:
		return "DebugTrapHeadsEveryCompound"
	case DebugTrapHeadsNone:
		return "DebugTrapHeadsNone"
	}
	return "DebugTrapHeadsWordAndArithmetic"
}

// debugCompoundHead fires the trap for a command that is not a simple one,
// where this dialect's reading says a head fires at all.
//
// One site rather than one per clause, because the two readings divide the
// node kinds between them and a rule spread over nine handlers is a rule that
// drifts. The two heads that fire *per pass* rather than once are the
// exception and are fired from inside their own loops — see debugPass.
func (r *Runner) debugCompoundHead(ctx context.Context, c syntax.Command) {
	switch r.sem().DebugTrapCompoundHeads {
	case DebugTrapHeadsNone:
		return
	case DebugTrapHeadsEveryCompound:
		if _, simple := c.(*syntax.SimpleCmd); simple {
			// A simple command fires from its own site, which is where the
			// measurements put it: before anything about the command is
			// expanded.
			return
		}
	default:
		switch c.(type) {
		case *syntax.CaseClause, *syntax.TestClause, *syntax.ArithCmdClause, *syntax.SelectClause:
		default:
			return
		}
	}
	r.runDebugTrap(ctx)
}

// debugPass fires the trap at a head that fires more than once — the list
// `for`'s head on every pass, and each evaluation of an arithmetic `for`'s
// three expressions — naming the head's own line rather than wherever the
// body has got to.
//
// pos is the head's position, because a pass after the first fires with the
// line record sitting on the body's last command. The record is put back
// afterward, so nothing about the body's own reporting moves.
func (r *Runner) debugPass(ctx context.Context, pos syntax.Pos) {
	if r.sem().DebugTrapCompoundHeads != DebugTrapHeadsWordAndArithmetic {
		return
	}
	line := r.line
	r.line = r.lineOf(pos)
	r.runDebugTrap(ctx)
	r.line = line
}

// debugArithPart fires the trap for one of the arithmetic `for`'s three
// expressions, which the reading that counts them counts one at a time: a
// two-pass loop writes the initializer, three conditions, two steps and two
// bodies.
//
// A part the script did not write fires nothing, which is what keeps
// `for ((;;))` silent between its passes.
func (r *Runner) debugArithPart(ctx context.Context, pos syntax.Pos, tree syntax.ArithExpr, text string) {
	if tree == nil && strings.TrimSpace(text) == "" {
		return
	}
	r.debugPass(ctx, pos)
}

// debugFunctionEntry fires the trap as a call enters the body, for the one
// column that writes one there.
//
// The line is the body's opening rather than the definition's: measured
// 2026-09-13 on bash 5.3.15 with the shell tracing calls, a function whose
// `()` is on line 3 and whose `{` is on line 4 writes the entry head at 4.
// ksh93 and zsh write none at all, which is why this is its own answer rather
// than a fourth reading of DebugTrapHeads — a function body is a group, and
// the column that writes a head for a group at the top level is not the one
// that writes this.
func (r *Runner) debugFunctionEntry(ctx context.Context, fn *syntax.FuncDecl) {
	// Nothing to ask where there is no trap to run. Every call reaches this,
	// and an axis put to a dialect that has no DEBUG condition at all is a
	// refusal on a question the script never asked — measured the hard way:
	// the complaint came out of `f() { local x; }` in the two columns with
	// no DEBUG trap to fire.
	if fn.Body == nil || r.debugTrap == nil || *r.debugTrap == "" {
		return
	}
	if !r.ask(r.sem().DebugTrapFiresOnEnteringAFunction,
		"a DEBUG trap as a call enters a function's body") {
		return
	}
	line := r.line
	r.line = r.lineOf(fn.Body.Pos())
	r.runDebugTrap(ctx)
	r.line = line
}
