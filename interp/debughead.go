// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"

	"github.com/blairham/sh/syntax"
)

// Which commands other than the simple ones fire the DEBUG trap.
//
// Every column that has the condition fires it before a simple command, and
// this engine fired it there and nowhere else — with a comment at the firing
// site saying a compound heading fires nothing, which is measured to be wrong
// in all four of the columns that have a DEBUG trap at all.
//
// Measured 2026-09-13 with `trap 'echo D $LINENO' DEBUG` over scripts of one
// construct per line, each construct read off the count of D lines and the
// lines they named:
//
//	construct              bash 5.3 / as sh / 3.2   ksh93               zsh
//	for w in a b           the head on every pass   the head per pass   once
//	for w in               nothing, it never runs   nothing             once
//	select                 the head once            the head per pass   once
//	case                   the head once            once                once
//	[[ … ]]                the head once            once                once
//	(( … ))                the head once            once                once
//	for ((i;c;n))          three parts per round    the written ones    once
//	if / while / until     nothing                  nothing             once
//	{ … } / ( … )          nothing                  nothing             once
//	f() { … } (defining)   nothing                  nothing             once
//	repeat                 no such construct        no such construct   once
//
// So the panel gives three readings rather than a rule with exceptions, and
// which one a dialect takes cannot be derived from anything else it answers.
// dash and BusyBox ash have no DEBUG condition and never reach the question.
//
// Two of ksh93's three departures from the bash columns are modeled here and
// the third is recorded rather than modeled: a `for` or `select` head that
// fires again on a later pass names **wherever the line record has got to** —
// the body's last line — where the bash columns name the head's own line on
// every pass. Reproducing that would mean a line rule that is right for the
// list loops and wrong for the arithmetic one, which names the head's line on
// every pass in ksh93 too.

// DebugTrapHeads is which compound commands fire the DEBUG trap.
type DebugTrapHeads int

const (
	// DebugTrapHeadsWordAndArithmetic fires at the heads whose own work is
	// expansion or arithmetic rather than a command: `case`, `[[`, `((` and
	// `select` once each, the list `for` on every pass, and each of the
	// arithmetic `for`'s three expressions every time one is evaluated,
	// whether the script wrote that expression or not. Nothing else. bash
	// 5.3, that build invoked as `sh`, and bash 3.2, so it is the zero
	// value.
	//
	// `for` firing per pass where `select` fires once is measured and not a
	// slip: a two-reply `select` writes one head and two bodies, and a
	// two-item `for` writes two heads and two bodies.
	DebugTrapHeadsWordAndArithmetic DebugTrapHeads = iota
	// DebugTrapHeadsEveryPassAndWrittenParts fires at the same heads, and
	// parts from the bash reading in two measured ways: **every** loop head
	// that fires at all fires once per pass, the menu loop's included, and
	// an arithmetic `for`'s initializer or step fires nothing when the
	// script did not write one. Its condition fires whether it was written
	// or not, which is what keeps `for ((;;))` writing one head per round
	// there and two in the bash columns. ksh93.
	DebugTrapHeadsEveryPassAndWrittenParts
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
	case DebugTrapHeadsEveryPassAndWrittenParts:
		return "DebugTrapHeadsEveryPassAndWrittenParts"
	case DebugTrapHeadsEveryCompound:
		return "DebugTrapHeadsEveryCompound"
	case DebugTrapHeadsNone:
		return "DebugTrapHeadsNone"
	}
	return "DebugTrapHeadsWordAndArithmetic"
}

// headsPerPass reports whether a loop head repeats with the loop's passes,
// which is the two readings that count passes at all.
func (d DebugTrapHeads) headsPerPass() bool {
	return d == DebugTrapHeadsWordAndArithmetic || d == DebugTrapHeadsEveryPassAndWrittenParts
}

// debugCompoundHead fires the trap for a command that is not a simple one,
// where this dialect's reading says a head fires at all.
//
// One site rather than one per clause, because the readings divide the node
// kinds between them and a rule spread over a dozen handlers is a rule that
// drifts. The heads that fire *per pass* rather than once are the exception
// and fire from inside their own loops — see debugPass.
func (r *Runner) debugCompoundHead(ctx context.Context, c syntax.Command) {
	switch heads := r.sem().DebugTrapCompoundHeads; heads {
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
		case *syntax.CaseClause, *syntax.TestClause, *syntax.ArithCmdClause:
		case *syntax.SelectClause:
			if heads == DebugTrapHeadsEveryPassAndWrittenParts {
				// A menu loop's head is one of the per-pass heads in this
				// reading, so it fires from inside the loop instead — see
				// selectClause.
				return
			}
		default:
			return
		}
	}
	r.runDebugTrap(ctx)
}

// debugPass fires the trap at a head that fires more than once — the list
// `for`'s head on every pass, the menu loop's in the reading that counts it,
// and each evaluation of an arithmetic `for`'s three expressions — naming the
// head's own line rather than wherever the body has got to.
//
// pos is the head's position, because a pass after the first fires with the
// line record sitting on the body's last command. The record is put back
// afterward, so nothing about the body's own reporting moves.
func (r *Runner) debugPass(ctx context.Context, pos syntax.Pos) {
	if !r.sem().DebugTrapCompoundHeads.headsPerPass() {
		return
	}
	line := r.line
	r.line = r.lineOf(pos)
	r.runDebugTrap(ctx)
	r.line = line
}

// debugSelectPass fires a menu loop's head for the one reading that repeats
// it with the replies. The other two fire it once, from the dispatcher.
func (r *Runner) debugSelectPass(ctx context.Context, pos syntax.Pos) {
	if r.sem().DebugTrapCompoundHeads != DebugTrapHeadsEveryPassAndWrittenParts {
		return
	}
	r.debugPass(ctx, pos)
}

// debugArithPart fires the trap for the arithmetic `for`'s initializer or its
// step, which one reading counts only where the script wrote one.
//
// The condition is not one of these: it fires under both readings whether it
// was written or not, so it calls debugPass directly. Measured — `for ((;;))`
// writes one head per round in the reading that skips the unwritten parts and
// two in the one that does not, and both of those counts are the same for
// every combination of the three parts a script can leave out.
func (r *Runner) debugArithPart(ctx context.Context, pos syntax.Pos, written bool) {
	if !written && r.sem().DebugTrapCompoundHeads == DebugTrapHeadsEveryPassAndWrittenParts {
		return
	}
	r.debugPass(ctx, pos)
}
