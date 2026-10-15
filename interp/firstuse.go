// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"maps"
	"slices"
)

// A **first-use value** is one a front end gives the shell to store under a
// name, and that the shell stores the first time anything touches the name
// rather than when it is given.
//
// It exists for one default that costs a start more than the start can
// afford (#6482). zsh's `$fpath` default names another installation's
// function library, and finding that library means walking a directory tree:
// on Debian's layout it was 1.5 ms of a 4.5 ms `zsh -c :` on the CI runner,
// and 0.46 ms after #6495 made the walk list three directories instead of
// twenty-nine. Most starts never ask. A `-c` script, a subshell, a script
// file that does not autoload anything — none of them reads `$fpath`, and
// each was paying for it.
//
// What a script can see is unchanged, and that is the whole contract. Every
// path to a name runs through the store, so the value is stored by the first
// read, write, `unset`, attribute question or listing of the name or of the
// other half of its tie, before that operation looks at the cell: whatever
// the operation would have seen with the value stored at the start, it sees.
// A listing or a save of the whole store lays every waiting value down first.
//
// It is laid down as the global value, which is what it would have been. The
// first touch of a name is always of the outermost binding: a `local` is
// itself a touch, and it touches the name before making the local, so no
// function can shadow a name that is still waiting.
//
// A clone carries the waiting state with it and settles it on its own: the
// flag is in the cell, and the cells are the clone's copy. The function the
// value comes from may therefore be called once per runner that touches the
// name, and a caller wanting the work done once per process says so in the
// function (driver memoizes the walk).

// firstUseSeed is one name's waiting value: the name it is stored under, the
// names whose touch lays it down (that name and its tie partner), and where
// the value comes from. ok false means there is no value after all, and the
// name keeps whatever it held.
type firstUseSeed struct {
	name    string
	touches []string
	value   func() (string, bool)
}

// SetVarOnFirstUse arranges for value's answer to be stored under name, as
// [Runner.SetVar] would store it, the first time anything touches name or the
// other half of a tie name is in. Tie first: the partner is found when this is
// called.
//
// For a front end, at seeding time and before any script runs, and only for a
// default whose cost a start should not pay — see the top of this file.
func (r *Runner) SetVarOnFirstUse(name string, value func() (string, bool)) {
	seed := &firstUseSeed{name: name, touches: []string{name}, value: value}
	if t, ok := r.tieOf(name); ok {
		for _, n := range [...]string{t.scalar, t.array} {
			if !slices.Contains(seed.touches, n) {
				seed.touches = append(seed.touches, n)
			}
		}
	}
	// A copy, so that a clone that still shares the old map never sees a
	// seed its parent added after it was made.
	seeds := maps.Clone(r.firstUseSeeds)
	if seeds == nil {
		seeds = map[string]*firstUseSeed{}
	}
	// Every cell made first and flagged after, so that making the second
	// cannot find the first already waiting and settle it on the spot.
	for _, n := range seed.touches {
		seeds[n] = seed
		r.cellFor(n)
	}
	for _, n := range seed.touches {
		if i := r.cellIndex[n]; !r.cellSlab[i].firstUse {
			r.cellSlab[i].firstUse = true
			r.firstUsePending++
		}
	}
	r.firstUseSeeds = seeds
	// cellFor remembered the cell as the last one found, and the shortcut
	// that answers from that memory does not look at the flag.
	r.lastCellName, r.lastCell = "", 0
}

// settleFirstUse lays down the value waiting under a name that was touched.
// Every name the seed watches stops waiting first, so that storing the value
// — which writes the name, and the other half through the tie — is an
// ordinary write.
func (r *Runner) settleFirstUse(name string) {
	seed := r.firstUseSeeds[name]
	if seed == nil {
		r.clearFirstUse(name)
		return
	}
	for _, n := range seed.touches {
		r.clearFirstUse(n)
	}
	if v, ok := seed.value(); ok {
		storeFirstUse(r, seed.name, v)
	}
}

// storeFirstUse is setVar, reached through a variable: the store calls
// settleFirstUse, and setVar reaches the builtins table, so naming it directly
// would make the table's initialization depend on itself.
var storeFirstUse func(r *Runner, name, value string)

func init() { storeFirstUse = (*Runner).setVar }

// clearFirstUse takes the waiting flag off a name's cell, found by position
// rather than through cell, which would settle it.
func (r *Runner) clearFirstUse(name string) {
	if i, ok := r.cellIndex[name]; ok && r.cellSlab[i].firstUse {
		r.cellSlab[i].firstUse = false
		r.firstUsePending--
	}
}

// settleEveryFirstUse lays down every waiting value, for an operation about
// to look at the whole store.
func (r *Runner) settleEveryFirstUse() {
	for r.firstUsePending > 0 {
		settled := false
		for i := range r.cellSlab {
			if r.cellSlab[i].firstUse {
				r.settleFirstUse(r.cellSlab[i].name)
				settled = true
				break
			}
		}
		if !settled {
			// The count and the flags disagree, which nothing should be
			// able to do; trust the flags rather than loop.
			r.firstUsePending = 0
		}
	}
}
