// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

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
func (r *Runner) expandArrayOperands() {
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
			r.literalReadsSubscripts(a.Name, a.Elems, a.Append))
		if !ok {
			// The element list failed — an unmatched pattern where the
			// dialect calls that an error, a division by zero. The store
			// abandons the assignment for the same reason, so there is
			// nothing left for it to take.
			continue
		}
		op.expanded = &expandedAssign{assign: a, elems: parsed, elemsSet: true}
	}
}
