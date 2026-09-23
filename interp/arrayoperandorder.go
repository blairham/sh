// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
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
func (r *Runner) expandArrayOperands(tableLetterAhead bool) {
	for i := range r.arrayOperands {
		op := &r.arrayOperands[i]
		a := op.assign
		if a.Members != nil || len(a.Elems) == 0 {
			// A compound body writes the member assignments it performs and
			// has no element list of its own, and an empty literal has
			// nothing to expand.
			continue
		}
		parsed, ok := r.literalElems(a.Elems,
			r.literalReadsSubscripts(a.Name, a.Elems, a.Append),
			r.bareLiteralElementIsOneValue(a.Name, a.Elems, tableLetterAhead))
		if !ok {
			// The element list failed — an unmatched pattern where the
			// dialect calls that an error, a division by zero. The store
			// abandons the assignment for the same reason, so there is
			// nothing left for it to take.
			continue
		}
		op.expanded = &expandedAssign{
			assign: a, elems: parsed, elemsSet: true, elemsAreOperands: true,
		}
	}
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
