// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// A grammar option moved at run time replaces the runner's dialect with a
// copy rather than writing through it, because the pointer is shared with
// every subshell. The copy is the same value every time the same option is
// moved the same way from the same dialect, so the runner keeps the pointer
// it made and hands it back.
//
// That matters for more than the copy. A function that opens with
// `emulate -L zsh` and sets `rc_quotes` or `no_short_loops` moves three or
// four of these on the way in and puts the caller's pointer back on the way
// out, so without this every call ran on dialect pointers nothing had seen
// before — about fifty fresh copies per F-Sy-H highlight — and anything
// keyed on the pointer, the arithmetic span cache first, never saw the same
// grammar twice (#6423).
//
// The kept pointers are the runner's own and are not handed to a clone: what
// a copy holds was built by this runner's dialect(), whose readers are bound
// to it.
type dialectMove uint8

const (
	moveDoubledQuote dialectMove = iota
	moveSubstitutionBodyCondition
	moveShortRepeatBody
	moveShortFormBody
	moveFunctionKeywordBody
	moveLoopBodyEndsInEnd
	moveQuotedNewlineIsUnmatched
	moveExtendedPattern
)

// dialectMoveKey is one move from one dialect, with the state of the runner
// that dialect() reads beside the pointer: the arithmetic order, which the
// copy carries.
type dialectMoveKey struct {
	from  *syntax.Dialect
	move  dialectMove
	on    bool
	prec  syntax.ArithPrecedencePolicy
	moved bool
}

// dialectMovesKept bounds the table. Moves from pointers this table made
// close over a handful of option combinations; a pointer made by a move that
// does not come through here is fresh every time, and would grow it forever.
const dialectMovesKept = 64

// moveDialect replaces r.Dialect with it moved by m to on.
func (r *Runner) moveDialect(m dialectMove, on bool) {
	k := dialectMoveKey{from: r.Dialect, move: m, on: on, prec: r.arithPrecedence, moved: r.arithPrecedenceMoved}
	if d, ok := r.dialectMoves[k]; ok {
		r.Dialect = d
		return
	}
	d := r.dialect()
	switch m {
	case moveDoubledQuote:
		d.DoubledQuoteInSingleQuotesIsALiteralQuote = on
	case moveSubstitutionBodyCondition:
		d.SubstitutionBodyRefusesAnUnfinishedCondition = on
	case moveShortRepeatBody:
		d.ShortRepeatBody = on
	case moveShortFormBody:
		d.ShortFormBody = on
	case moveFunctionKeywordBody:
		d.FunctionKeywordBodyIsOptional = on
		d.FunctionKeywordBodyMustBeBraceGroup = !on
	case moveLoopBodyEndsInEnd:
		d.LoopBodyEndsInEnd = on
	case moveQuotedNewlineIsUnmatched:
		d.QuotedNewlineIsUnmatched = on
	case moveExtendedPattern:
		d.ExtendedPattern = on
	}
	if r.dialectMoves == nil || len(r.dialectMoves) >= dialectMovesKept {
		r.dialectMoves = make(map[dialectMoveKey]*syntax.Dialect, 8)
	}
	r.dialectMoves[k] = &d
	r.Dialect = &d
}
