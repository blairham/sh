// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"maps"
	"slices"
)

// What a cloned Runner owns, and why the list is checked rather than trusted.
//
// clone starts from `c := *r`, which copies every field by value — and a map
// held by value is a reference, so a table left out of this file is one map
// with two shells writing it. That has two consequences and only one of them
// is visible:
//
//   - A subshell's change leaks into the parent. `x=1; (readonly x); x=2`
//     refused the assignment because `readonly` was shared, where every shell
//     in the panel assigns 2 (#1384).
//   - A *concurrent* shell corrupts the runtime's map. A process substitution
//     is a clone that runs on a goroutine, so the parent and the child write
//     one table with no lock at all. `removed` was the one that fired:
//     `nameIsBack` deletes from it on every assignment and `unset` writes it,
//     while a child's `environ()` reads it through `isExported` on every
//     external command. That ends the process with `fatal error: concurrent
//     map read and map write`, which is not a Go panic — the runtime's
//     detector does not raise one — so panicguard cannot catch it and every
//     recovery seam in the tree is bypassed. Exit status 2, no diagnostic.
//
// The second is why this is one function with a test behind it rather than a
// dozen lines inside clone. The enumeration was already twelve tables long
// and looked complete; twenty-two were missing, and nothing could say so,
// because a shared map is invisible until two shells touch the same name.
// TestACloneOwnsEveryTable walks Runner by reflection, so a table added to
// the struct fails a test until somebody decides which side of this it is on.
//
// It also has to be seeded to be seen, which is the trap that hid `removed`
// for so long: **maps.Clone keeps a nil map nil**, and every writer here
// allocates lazily. A parent that has never written a table hands the clone a
// nil, the clone allocates one of its own, and the leak does not exist yet.
// So the alias half of this hid until the parent had an alias, and the
// `removed` crash needs an `unset` *before* the substitution — with none, the
// clone's own `unset` builds the map the parent's field never pointed at and
// nothing races. The test seeds every table for that reason.
func (c *Runner) ownTables(r *Runner) {
	// The variable stores. Vars and exported are made rather than cloned
	// because a nil one is not safe to write and these two are written
	// without a lazy allocation in front of them.
	c.Vars = make(map[string]string, len(r.Vars))
	for k, v := range r.Vars {
		c.Vars[k] = v
	}
	c.exported = make(map[string]bool, len(r.exported))
	for k, v := range r.exported {
		c.exported[k] = v
	}
	// The two that hold a container per name, so the element has to be
	// copied too: cloning the outer map alone would give the subshell its
	// own name table pointing at the parent's elements.
	c.Arrays = make(map[string]Array, len(r.Arrays))
	for k, v := range r.Arrays {
		c.Arrays[k] = maps.Clone(v)
	}
	c.AssocArrays = make(map[string]AssocArray, len(r.AssocArrays))
	for k, v := range r.AssocArrays {
		c.AssocArrays[k] = maps.Clone(v)
	}
	c.Params = append([]string(nil), r.Params...)

	// What the shell knows about a name besides its value. Each is written
	// on an ordinary assignment or by `unset`, which is what puts them on
	// the hot path a process substitution shares with its parent:
	//
	//	removed        `unset` took the name away, so it is no longer
	//	               exported and no longer falls back to the environment
	//	assigned       the assignment a `${x=…}` and friends recorded
	//	absentParams   a name a nounset check has already spoken about
	//	declaredEmpty  declared with no value, which is not the same as unset
	c.removed = maps.Clone(r.removed)
	c.assigned = maps.Clone(r.assigned)
	c.absentParams = maps.Clone(r.absentParams)
	c.declaredEmpty = maps.Clone(r.declaredEmpty)

	// The attribute tables `declare` and `typeset` write. A subshell's
	// attribute must not outlive it: measured, `x=1; (readonly x); x=2`
	// assigns 2 in bash, ksh93 and zsh, and `n=5; (typeset -i n); n=1+1`
	// leaves the three characters everywhere.
	c.readonly = maps.Clone(r.readonly)
	c.integer = maps.Clone(r.integer)
	c.integerBase = maps.Clone(r.integerBase)
	c.floatPrecision = maps.Clone(r.floatPrecision)
	c.lowered = maps.Clone(r.lowered)
	c.uppered = maps.Clone(r.uppered)
	c.hidden = maps.Clone(r.hidden)
	c.unique = maps.Clone(r.unique)
	c.hideInScope = maps.Clone(r.hideInScope)
	c.tied = maps.Clone(r.tied)
	// freezing, literalOperands and declaring are only ever *replaced* as a
	// whole, which makes sharing them harmless today and makes cloning them
	// free. They are here because the rule is the struct field and not the
	// current set of writers: a keyed write added to one of them later is a
	// change nobody would think to look at this file for.
	c.freezing = maps.Clone(r.freezing)
	c.literalOperands = maps.Clone(r.literalOperands)
	// indexedLetterHere *is* written by key, one name at a time, so it is
	// here on the stronger footing than the three above it.
	c.indexedLetterHere = maps.Clone(r.indexedLetterHere)
	c.declaring = maps.Clone(r.declaring)
	c.precommands = maps.Clone(r.precommands)

	// The option table, on the same terms: `(setopt …)` is the subshell's.
	c.extraOptions = maps.Clone(r.extraOptions)

	// The descriptor tables are copied and the streams in them are shared: a
	// subshell's `exec 7>&1` must not appear in the parent, and its writes
	// through a descriptor the parent made must still land where the parent
	// pointed it. execFds is the record of which of them `exec` marked, so
	// it travels with fds or it describes the wrong table.
	c.fds = maps.Clone(r.fds)
	c.execFds = maps.Clone(r.execFds)
	// And on the same terms, for the same reason: a subshell's `sysopen -o
	// cloexec` must not decide what the parent hands to a child.
	c.cloexecFds = maps.Clone(r.cloexecFds)

	c.completions = maps.Clone(r.completions)

	// The tables that say what a command *name* means, on the same terms as
	// the variable tables above: a subshell inherits them and owns what it
	// then does to them. All four shells in the panel agree, and they agree
	// in both directions — `(g(){ :; }); type g` finds nothing afterwards,
	// and `f(){ :; }; (unset -f f); type f` still finds `f`. Sharing the maps
	// made a definition made in a subshell the parent's, and a removal made
	// in one the parent's too, at status 0 with nothing said either way.
	c.funcs = maps.Clone(r.funcs)
	// And the math-function registrations, which are the same kind of table
	// under a second name: a `functions -M` made inside a subshell is not
	// the parent's afterwards, and one the parent made is the subshell's to
	// call. mathOrder is a slice and is copied outright, so a removal in the
	// subshell cannot shorten the parent's.
	c.mathFuncs = maps.Clone(r.mathFuncs)
	c.mathOrder = append([]string(nil), r.mathOrder...)
	c.funcFiles = maps.Clone(r.funcFiles)
	c.exportedFuncs = maps.Clone(r.exportedFuncs)
	c.aliases = maps.Clone(r.aliases)
	c.suffixAliases = maps.Clone(r.suffixAliases)
	c.disabledBuiltins = maps.Clone(r.disabledBuiltins)
	c.withdrawnBuiltins = maps.Clone(r.withdrawnBuiltins)
	// And the parameter half, which has to travel with the tables it takes
	// names out of: those are cloned below, so a withdrawal that stayed
	// shared would put a name back into the wrong shell's tables.
	c.withdrawnParams = maps.Clone(r.withdrawnParams)

	// The extension points. An embedder registers these before a run and a
	// dialect's Apply is the usual caller, so nothing a *script* does writes
	// one — but the shell itself does: Dynamic is built lazily, and `_` and
	// `LINENO` are installed into it on the way past. That write is on the
	// same goroutine question as every table above, and a substitution
	// looking up a variable is enough to make it.
	c.Dynamic = maps.Clone(r.Dynamic)
	c.DynamicArrays = maps.Clone(r.DynamicArrays)
	c.DynamicAssocs = maps.Clone(r.DynamicAssocs)
	c.dynamicAssocElements = maps.Clone(r.dynamicAssocElements)
	c.dynamicAssocWriters = maps.Clone(r.dynamicAssocWriters)
	// And the array writer travels with DynamicArrays for the same reason
	// the table above travels with DynamicAssocs.
	c.dynamicArrayWriters = maps.Clone(r.dynamicArrayWriters)
	// absentElements travels with DynamicAssocs and the two tables beside
	// it — the keyed reading and the writer — and copying some of that group
	// and not the rest would be the split dynamicWriters describes below: a subshell owning the producer while sharing the sentence a key
	// it cannot answer is refused with. It is also the element half of
	// absentParams, which is copied above, and one mechanism should not have
	// two answers about whose it is.
	c.absentElements = maps.Clone(r.absentElements)
	// dynamicWriters travels with Dynamic, and sharing it alone would be
	// worse than sharing either. UnsetDynamic ends a produced parameter by
	// deleting from several tables at once — Dynamic, this one, assigned, and
	// whatever else is added to it later — and every other one of them is
	// copied here. So a subshell ending such a parameter would take the
	// parent's
	// writer while leaving the parent's producer: the name still answers
	// every read, and an assignment to it lands nowhere and says nothing,
	// which is the silent-write case SetDynamicWriter exists to prevent.
	// The call-scoped parameters its doc comment describes are a line
	// editor's, so a widget running in a subshell is the live path.
	c.dynamicWriters = maps.Clone(r.dynamicWriters)
	c.custom = maps.Clone(r.custom)

	// The stacks, for the same two reasons as the tables above and a third
	// that only a slice has.
	//
	// A slice header copied by value carries the parent's *backing array*, and
	// `append` writes that array in place whenever it has the room. Two shells
	// appending to one stack therefore write one array and then both read back
	// `[len-1]` and get the same element — not a leaked value but an aliased
	// slot. `scopes` is the one that ends the process, because the element is a
	// `*scope` and a scope holds fourteen maps of its own.
	//
	// The third reason is why this did not surface with #1384: a nil slice is
	// safe, because two clones each allocate on first append. Nothing aliases
	// until the parent has pushed and popped once, and popping keeps the
	// capacity — `popFrame` is `r.frames = r.frames[:len(r.frames)-1]`. So the
	// *second* pipeline in a script is the first one that aliases, which is
	// exactly what was measured for #1783: `f | f` once is clean, and twice is
	// a data race in `pushFrame`.
	//
	// slices.Clone keeps a nil slice nil, so the same seeding trap applies as
	// for the tables and the test seeds these too.
	c.frames = slices.Clone(r.frames)
	// The same reason, for the same shape: `runSourced` appends to this and
	// truncates it back, so a subshell started from inside a sourced file
	// would share the array and both would write [len-1].
	c.borrowed = slices.Clone(r.borrowed)
	c.scopes = cloneScopes(r.scopes)
	// Appended to in place as well, so each needs an array of its own. Their
	// *elements* stay shared on purpose: a `*Job` is one job to whoever holds
	// it, and copying the pointer is what keeps a subshell looking at the
	// parent's job rather than a snapshot of it.
	c.redirFds = slices.Clone(r.redirFds)
	c.jobs = slices.Clone(r.jobs)
	c.jobOrder = slices.Clone(r.jobOrder)
	c.aroundFunctionCalls = slices.Clone(r.aroundFunctionCalls)
	c.freezeAfter = slices.Clone(r.freezeAfter)

	// traps, inheritedIgnored and selfPending are deliberately not here.
	// inheritTraps builds all three from scratch immediately after this
	// runs, because what a subshell starts with is not the parent's table
	// filtered by this file's rule — it is POSIX's rule about handled and
	// ignored conditions, which is a behavior and lives with the behavior.
	// Cloning them here would be a copy something else then throws away.
	//
	// preludeFuncs is deliberately not here either, and that one is a claim
	// rather than a hand-off. It records what the dialect's own shell text
	// declared, is written only while the prelude is being sourced, and is
	// never deleted from — so there is no moment at which a subshell could
	// change it and no moment at which one could be running. What a script
	// *did* to one of those names is in funcs, which is copied; this is only
	// how the shell recognizes its own declaration when it sees it again.
	// sharedTables in the test names it, so the claim is asserted rather
	// than merely written down here.
}

// cloneScopes gives a clone its own scope stack, scopes and all.
//
// Every scope is copied and not only the slice, because a scope's tables are
// inside the scope rather than on the Runner: a fresh backing array alone
// would still hand two shells one `*scope`, and `shadow` writes
// `sc.saved[name]` on whatever it finds at the top of the stack. That is the
// `fatal error: concurrent map writes` of #1783 — the runtime's own detector,
// which is not a Go panic, so panicguard cannot catch it.
//
// owner is copied verbatim rather than repointed at the clone. It records
// which runner *pushed* the scope, and ownScope reads it to stop a `( … )`
// written in a function body from writing the caller's scope — see
// localtraps.go, where the measured case is a subshell's `trap` not
// surviving into the caller. Repointing it would make every inherited scope
// look like the subshell's own and change what that check answers, which is
// a behavior question this file does not get to decide. Here only the
// memory changes hands: what the shell *does* with a scope is unchanged, and
// the axis that decides whether the last pipeline element runs in the current
// shell at all still decides it (see pipeline.go).
func cloneScopes(scopes []*scope) []*scope {
	if scopes == nil {
		return nil
	}
	out := make([]*scope, len(scopes))
	for i, sc := range scopes {
		c := *sc
		c.saved = maps.Clone(sc.saved)
		c.existed = maps.Clone(sc.existed)
		c.arrayExisted = maps.Clone(sc.arrayExisted)
		c.removedBefore = maps.Clone(sc.removedBefore)
		c.assocExisted = maps.Clone(sc.assocExisted)
		c.savedReadonly = maps.Clone(sc.savedReadonly)
		c.savedHideInScope = maps.Clone(sc.savedHideInScope)
		c.savedAttrs = maps.Clone(sc.savedAttrs)
		c.savedAssigned = maps.Clone(sc.savedAssigned)
		c.assignedSpoken = maps.Clone(sc.assignedSpoken)
		c.savedExported = maps.Clone(sc.savedExported)
		c.exportedSpoken = maps.Clone(sc.exportedSpoken)
		c.exportedShadow = maps.Clone(sc.exportedShadow)
		c.savedTraps = maps.Clone(sc.savedTraps)
		// The two that hold a container per name, on the same terms as Arrays
		// and AssocArrays above: cloning the outer map alone would give the
		// clone its own name table pointing at the parent's elements.
		if sc.savedArrays != nil {
			c.savedArrays = make(map[string]Array, len(sc.savedArrays))
			for k, v := range sc.savedArrays {
				c.savedArrays[k] = maps.Clone(v)
			}
		}
		if sc.savedAssoc != nil {
			c.savedAssoc = make(map[string]AssocArray, len(sc.savedAssoc))
			for k, v := range sc.savedAssoc {
				c.savedAssoc[k] = maps.Clone(v)
			}
		}
		// The return hooks are appended to in place too, by AtFunctionReturn
		// and by getopts. The closures themselves are shared, and they capture
		// the runner that registered them, so one registered by the parent
		// still unwinds the parent.
		c.onReturn = slices.Clone(sc.onReturn)
		out[i] = &c
	}
	return out
}
