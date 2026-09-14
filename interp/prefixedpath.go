// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"maps"
	"slices"

	"github.com/blairham/sh/syntax"
)

// A PATH an assignment prefix supplied, for the length of one command.
//
// `PATH=/nowhere ls` has two halves and this is the second of them. The child
// is handed the new PATH — that half has always worked, and `PATH=/x env`
// prints `/x` — and **the search this shell makes for the command is made
// with it too**, which is what makes the construct do anything at all:
// measured 2026-09-13, every column in the panel answers `PATH=/nowhere ls`
// with 127, and with two copies of one name on PATH, `PATH=$PWD/d2 zzc` runs
// d2's copy in all seven even after d1's was the one that ran a moment before.
// This shell ran the old one, or found it where the prefix said it would not
// be (#2626).
//
// It is applied to the shell rather than threaded into the search, because
// lookPath must go on having exactly one answer to "which PATH is this" — the
// same reason it reads this Runner's variables rather than the process's, see
// lookpath.go. So the prefix reaches PATH the way it reaches a name in front
// of a builtin: saved, set, and given back when the command is over.

// reachPrefixedPath puts a PATH an assignment prefix supplied on this shell
// and hands back the take-back.
//
// The command hash is held across the whole of it, because assigning PATH
// empties the table and the panel disagrees about whether *this* assignment
// counts as one. See holdCommandHashAcrossAPrefixedPath.
func (r *Runner) reachPrefixedPath(value string) func() {
	undo := []savedVar{r.saveVar("PATH")}
	takeBackHash := r.holdCommandHashAcrossAPrefixedPath()
	r.setVar("PATH", value)
	return func() {
		r.restoreVars(undo)
		takeBackHash()
	}
}

// holdCommandHashAcrossAPrefixedPath takes the command hash aside for a
// command whose PATH an assignment prefix supplied, and hands back the
// take-back: the table is either emptied or put back the way it was.
//
// The lookup itself needs no help — setting PATH empties the table, which is
// unanimous and is why `PATH=$PWD/d2 zzc` runs d2's copy in every column even
// with d1's remembered. What the panel splits on is what the table holds
// *afterwards*, and it is a 4-3 split measured 2026-09-13 with `zzc` hashed
// from d1 and then run again under `PATH=$PWD/d2`:
//
//	bash 5.3.15   the table is empty
//	bash-as-sh    empty
//	dash          empty
//	BusyBox ash   empty
//	bash 3.2.57   zzc, still d1's copy, at its old hit count
//	ksh93         zzc, still d1's copy
//	zsh 5.9.2     zzc, still d1's copy
//
// So four of them let the prefix reach the shell's own PATH — which empties
// the table, and putting the old value back does not refill it — and three
// keep the prefix out of the shell entirely, so the table they had is the
// table they still have. See Semantics.APrefixedPathEmptiesTheCommandHash.
//
// The three that keep it are the reason nothing here has to suppress the
// hashing the run does: an entry the prefixed search made is put back over,
// along with everything else, by the table this took aside.
//
// **Read rather than asked**, which is the same judgement lookPathReporting
// makes and for the same reason: `PATH=/x cmd` prints what it prints either
// way, and only a later `hash` can tell the readings apart. Refusing the
// command outright — which is what r.ask does where no dialect has answered —
// would refuse every `PATH=… cmd` in a shell nobody has told anything, to
// settle a side effect nothing in that command can see.
func (r *Runner) holdCommandHashAcrossAPrefixedPath() func() {
	// Copied rather than referred to, for the reason saveVar copies the
	// compound stores: both are live containers, and holding the ones that
	// are there holds a view of whatever the command then does to them.
	table, order := maps.Clone(r.cmdHash), slices.Clone(r.cmdHashOrder)
	return func() {
		if r.sem().APrefixedPathEmptiesTheCommandHash == Yes {
			r.forgetEveryHashedCommand()
			return
		}
		r.cmdHash, r.cmdHashOrder = table, order
	}
}

// aPrefixSuppliesThePath reports whether an assignment prefix assigns PATH.
//
// The three skips are the ones the routes that *apply* a prefix already make,
// said once: an operand assignment is an argument rather than a prefix, a
// numbered name is a positional parameter and never reaches the variable
// table, and a frozen name keeps what it holds. A prefix that assigns PATH in
// none of those ways is not one that supplies the path to search.
func (r *Runner) aPrefixSuppliesThePath(assigns []*syntax.Assign) bool {
	for _, a := range assigns {
		if a.Name != "PATH" || a.Operand || r.readonly[a.Name] {
			continue
		}
		if _, positional := positionalAssignIndex(a.Name); positional {
			continue
		}
		return true
	}
	return false
}
