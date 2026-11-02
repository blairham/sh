// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"strings"

	"github.com/blairham/sh/syntax"
)

// When a declaration utility's **array literal** operand is expanded, against
// when the command's own redirections are opened.
//
// `typeset a=(…)` is one command carrying one assignment, and the assignment
// is applied after the utility has run — which is what lets `local` decide the
// scope the elements land in, and is why Runner.assignOperands stands where it
// does. Expanding the elements there as well put the expansion *inside* the
// command's redirections, so a substitution's standard error went wherever the
// command sent it.
//
// Measured 2026-09-19, `env -i PATH=/usr/bin:/bin LC_ALL=C <shell> x.sh` over
// a script file, stdin on /dev/null, for
//
//	typeset a=($(echo hi >&2)) 2>/dev/null
//	echo done
//
//	bash 5.3.20        	`hi` then `done`
//	bash 3.2.57        	`hi` then `done`
//	ksh93u+ 2012-08-01 	`hi` then `done`
//	zsh 5.9.2          	`done`
//	dash 0.5.12        	no array literal — the question cannot be put
//	BusyBox ash 1.37.0 	no array literal — the question cannot be put
//
// So the two columns that can be asked both expand the elements before they
// open anything, and there is no disagreement to record as an axis.
//
// **zsh's column is not a second answer**, and the control says so: the same
// redirection swallows a *plain* assignment's substitution there too —
// `v=$(echo hi >&2) 2>/dev/null` and `typeset s=$(echo hi >&2) 2>/dev/null`
// are silent in zsh and escape in bash and ksh93 — so what zsh answers
// differently is where a command's redirections stand against its assignments
// in general. This engine does not model that difference for any assignment,
// and modeling it is not this file's question; until it is modeled, zsh's
// array operand behaves here exactly as its scalar one does.
//
// A stronger probe than the swallowed one, because it separates the expansion
// from the store: with `2>/nope/x` in place of `2>/dev/null`, bash and ksh93
// still write `hi` and the redirection *then* fails, so the elements were
// expanded before the open was even attempted. Whether the assignment is still
// applied once the open has failed is a further question — bash applies it and
// this shell does not — and it is not answered here.
//
// Nothing is expanded twice: the list is handed to the store that runs after
// the utility, through Runner.expandedArrayOperand and Runner.withPreparedValue,
// which is the arrangement a bare literal's trace has used since #1915. And
// nothing about *how* the elements are read moves with them — whether a
// `[sub]=value` element names where its value goes is settled by the grammar
// and by what the name is already holding, and the `-A` question is still
// answered by the store. See interp/xtracearrayoperand.go, which measured that
// when the same expansion was moved here for the trace alone (#3567).

// expandArrayOperands expands each array-literal operand's elements once, at
// the point the command reaches — before its redirections are opened and
// before its own trace line is written.
//
// The elements are consumed by the store that runs after the utility, so this
// decides *when* the value was computed and nothing about what is stored.
// tableLetterAhead says the command these operands belong to carries the
// table letter, read off its own words because nothing has recorded it yet:
// the elements are expanded here, in front of the utility, and
// Runner.tableLetterHere is written by the utility. See
// Runner.bareLiteralElementIsOneValue, which is the one question that needs
// it.
//
// The operand walk reaches the literals in written order wherever there is
// one, and every column with the construct has one (#7075); the loop below is
// for a command it does not reach — a utility this shell does not have, or a
// literal over a frozen name in the column that stores every operand in turn.
func (r *Runner) expandArrayOperands(
	ctx context.Context,
	c *syntax.SimpleCmd,
	argv []string,
	turn **operandsInTurn,
	tableLetterAhead, indexedLetter bool,
) bool {
	if walked, goesOn := r.walkTheLiteralOperandsLeft(ctx, c, argv, turn); walked {
		return goesOn
	}
	for i := range r.arrayOperands {
		op := &r.arrayOperands[i]
		a := op.assign
		if op.expanded != nil {
			// Expanded already, ahead of a scalar operand written behind it.
			// See interp/declarationinturn.go.
			continue
		}
		if a.Members != nil {
			// A compound body writes the member assignments it performs and
			// has no element list of its own.
			continue
		}
		if len(a.Elems) == 0 {
			// An empty literal has nothing to expand, and its line is still
			// written here, where a populated one's is.
			r.traceArrayOperandAsItIsExpanded(op, tableLetterAhead)
			continue
		}
		e := &expandedAssign{assign: a, elemsAreOperands: true}
		if r.sem().DeclarationOperandsAreStoredInTurn == Yes {
			// The subscript's side first, read once for the trace and the
			// store alike. See Runner.resolveLiteralOperandSubscript.
			r.resolveLiteralOperandSubscript(a, e)
			if r.unspecified || r.givingUpAlready() {
				return false
			}
		}
		parsed, ok := r.literalElems(a.Elems,
			r.literalReadsSubscripts(a.Name, a.Elems, a.Append),
			r.bareLiteralElementIsOneValue(a.Name, a.Elems, tableLetterAhead))
		if !ok {
			// The element list failed — an unmatched pattern where the
			// dialect calls that an error, a division by zero — and the
			// command ends here, its later operands unexpanded. It used to
			// go on to the store, which expanded the same list a second
			// time: `typeset -a a=(5 $((1/0)) $(echo AFTER >&2))` reported
			// the division twice and ran `AFTER` (#6606). What a
			// subscripted literal had stored stays, in the column that
			// keeps it — and in the column whose container letter re-creates
			// the name, the name of that kind with nothing in it (#6705).
			how := failedLiteral{appendTo: a.Append, operand: true}
			switch {
			case tableLetterAhead:
				how.letter = 'A'
			case indexedLetter:
				how.letter = 'a'
			}
			r.keepWhatAFailedLiteralStored(a.Name, a.Elems, parsed, how)
			return false
		}
		e.elems, e.elemsSet = parsed, true
		op.expanded = e
		r.traceArrayOperandAsItIsExpanded(op, tableLetterAhead)
	}
	return true
}

// traceArrayOperandAsItIsExpanded writes an array-literal operand's lines as
// soon as its elements are expanded, in the dialects that take the operand off
// the command line, and marks it written so the command's own line does not
// write it again.
//
// Written here rather than with the command's line because that is where the
// shells write it: ahead of the prefix's lines in the column that reaches the
// operand first, and ahead of a later operand's failure. Measured 2026-10-30
// against GNU bash 5.3.20, script files under `env -i PATH=/usr/bin:/bin`
// (#7006):
//
//	set -x; v=new typeset -a a=(1)            `+ a=('1')`, `+ v=new`, `+ typeset -a a`
//	set -x; v=old; v=new typeset -a a=($v)    … `+ a=('old')`, `+ v=new`, …
//	set -x; v=1 w=2 declare -A m=([k]=v)      `+ m=(['k']='v')`, `+ v=1`, `+ w=2`, …
//	set -x; v=$(echo P >&2) typeset -a a=($(echo O >&2))
//	        the substitution `O`, `+ a=()`, then `P`, `+ v=`, the command
//	set -x; typeset -a a=(1) b=($((1/0)))     `+ a=('1')`, then the division
//
// ksh93u+ writes the literal's line at the same point, which there stands
// behind the prefix's, since that column works through the prefix first (see
// interp/prefixoperandorder.go): `set -x; v=$(echo P >&2) typeset -a
// a=($(echo O >&2))` is `P`, `+ v=”`, `O`, `+ a=( )`, `+ typeset -a a`.
func (r *Runner) traceArrayOperandAsItIsExpanded(op *arrayOperand, tableLetterAhead bool) {
	if op.traced || !r.tracing() {
		return
	}
	d := r.diag()
	if d.TraceDeclarationArrayOperand != TraceOperandSplitBefore {
		return
	}
	op.traced = true
	r.writeArrayOperandLines(*op, tableLetterAhead, d)
}

// walkTheLiteralOperandsLeft is the operand walk reaching the literals written
// behind the command's last word: each is expanded and traced in turn, and in
// the columns that place a literal before the operand behind it expands,
// placed — so `typeset -a a=(1) b=("${a[0]}")` gives `b` the 1 there, and a
// literal that fails leaves the ones in front of it in place: `typeset -a
// a=(5) b=(6 $((1/0)))` keeps `a=(5)` (#6798). Reports whether it walked
// them, and if so whether the command goes on.
//
// The walk that stores every operand leaves a literal over a frozen name to
// the refusal ahead of the command, which words it under the element where
// the column does — see Runner.refuseFrozenOperandValuesEarly — so it walks
// none of them there, as before.
func (r *Runner) walkTheLiteralOperandsLeft(
	ctx context.Context,
	c *syntax.SimpleCmd,
	argv []string,
	turn **operandsInTurn,
) (walked, goesOn bool) {
	if len(r.arrayOperands) == 0 {
		// Every command passes through here, and one with no literal operand
		// has nothing to walk and asks nothing.
		return false, true
	}
	t := *turn
	if t == nil {
		if t = r.startOperandsInTurn(c, argv); t == nil {
			return false, true
		}
		*turn = t
	}
	if t.stopped {
		return true, false
	}
	if !t.literalsOnly {
		for i := range r.arrayOperands {
			if r.attrHas(r.arrayOperands[i].assign.Name, attrReadonly) {
				return false, true
			}
		}
	}
	goesOn = r.settleLiteralOperandsBefore(ctx, c, nil, argv, t)
	for i := range r.arrayOperands {
		op := &r.arrayOperands[i]
		if e := t.literal(op.assign); e != nil {
			op.expanded = e
		}
		op.traced = op.traced || t.wasTraced(op.assign)
	}
	return true, goesOn
}

// indexedLetterAmongTheOptions reports whether a declaration utility's option
// words carry the indexed-array letter, read the same narrow way as the table
// letter below. Only a failed subscripted operand asks — see
// Runner.keepWhatAFailedLiteralStored.
func indexedLetterAmongTheOptions(argv []string) bool {
	for _, w := range argv[1:] {
		if w == "--" || len(w) < 2 || w[0] != '-' {
			return false
		}
		if strings.ContainsRune(w, 'a') {
			return true
		}
	}
	return false
}

// tableLetterAmongTheOptions reports whether a declaration utility's option
// words carry the table letter.
//
// Read off the words rather than parsed, and the narrowness is deliberate:
// the only thing it decides is which of two expansions a keyed literal's bare
// elements get, so a word it cannot make sense of is simply not a table
// letter and the elements are expanded the way they were before this existed.
// The utility's own option parse runs afterwards and is what reports anything
// wrong with them.
//
// Only ever asked of a command that already has an array-literal *operand*,
// which is a shape only a declaration utility has, so there is no question
// here of whether argv[0] is one.
func tableLetterAmongTheOptions(argv []string) bool {
	for _, w := range argv[1:] {
		if w == "--" {
			return false
		}
		if len(w) < 2 || (w[0] != '-' && w[0] != '+') {
			// The first operand: every option word is behind it.
			return false
		}
		if strings.ContainsRune(w, 'A') {
			return true
		}
	}
	return false
}

// containerLetterOverALiteral reports a `readonly` whose own words carry a
// container letter *and* an array-literal operand, which is the one shape
// that has to be declared before its value is stored.
//
// It decides the order of the two halves of such a command — see the branch
// in Runner.commandBuiltin that reads it. `readonly a=(x)` stores first,
// because freezing the name first would refuse the very assignment the
// command was given; but a container letter has to be *recorded* first, or
// the literal lands as whatever kind it looks like and the letter then meets
// a name of the other kind. `declare -Ar m=([k]=v)` has the same pair of
// needs and resolves them the other way round — the freeze waits — which is
// the order this borrows.
//
// Read off the words rather than parsed, the way tableLetterAmongTheOptions
// is, and with one extra condition: **every** option word has to be one this
// utility takes. An option it will refuse leaves the order alone, because the
// refusal comes before anything is stored in the borrowed order and after it
// in this one — measured 2026-09-23 on bash 5.3.20, `readonly -rA z=(k 1)` is
// `readonly: -r: invalid option` and then `declare -A z=([k]="1" )`, so the
// assignment survives the refusal there.
func (r *Runner) containerLetterOverALiteral(argv []string, c *syntax.SimpleCmd) bool {
	letters := r.sem().ReadonlyOptions
	container := false
	for _, w := range argv[1:] {
		if w == "--" {
			break
		}
		if len(w) < 2 || (w[0] != '-' && w[0] != '+') {
			// The first operand: every option word is behind it.
			break
		}
		for _, letter := range w[1:] {
			if !strings.ContainsRune(letters, letter) {
				return false
			}
			if letter == 'a' || letter == 'A' {
				container = true
			}
		}
	}
	if !container {
		return false
	}
	for _, a := range c.Assigns {
		if a.Name != "" && len(a.Elems) > 0 {
			return true
		}
	}
	return false
}

// An array literal and a later operand on the same name, in one declaration:
// `typeset g=() g[1]=yes g[2]=no` holds `yes no`, and `typeset y=(a b)
// y[1]=c` holds `c b` — the literal is stored where it was written, before
// the operand after it. Measured 2026-10-02 on zsh 5.9.2, B02typeset's `can
// set empty array` (#5142). The literal is otherwise stored once the utility
// has run, so that it lands in the scope the utility made; here it is stored
// as soon as the utility has declared its name and a later operand reaches
// the same name, which is the same scope a moment earlier.

// noteArrayOperandName records that the utility has reached the bare name an
// array operand left in its place.
func (r *Runner) noteArrayOperandName(name string) *syntax.Assign {
	for i := range r.arrayOperands {
		op := &r.arrayOperands[i]
		if !op.seen && op.assign.Name == name {
			op.seen = true
			return op.assign
		}
	}
	return nil
}

// storeArrayOperandBefore stores the array operand on base that the utility
// has already declared and not yet stored, ahead of a later operand on the
// same name.
func (r *Runner) storeArrayOperandBefore(base string) {
	for i := range r.arrayOperands {
		op := &r.arrayOperands[i]
		if !op.seen || op.applied || op.assign.Name != base {
			continue
		}
		op.applied = true
		declaring := r.writingADeclarationsOperand
		r.writingADeclarationsOperand = true
		if op.expanded != nil {
			r.withPreparedValue(r.ctx, op.expanded)
		} else {
			r.assign(r.ctx, op.assign)
		}
		r.writingADeclarationsOperand = declaring
	}
}

// arrayOperandApplied reports whether an array operand was stored already.
func (r *Runner) arrayOperandApplied(a *syntax.Assign) bool {
	for _, op := range r.arrayOperands {
		if op.assign == a {
			return op.applied
		}
	}
	return false
}
