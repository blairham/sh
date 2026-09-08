// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "maps"

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
	c.lowered = maps.Clone(r.lowered)
	c.uppered = maps.Clone(r.uppered)
	c.hidden = maps.Clone(r.hidden)
	c.unique = maps.Clone(r.unique)
	c.tied = maps.Clone(r.tied)
	// freezing and declaring are the two that are only ever *replaced* as a
	// whole, which makes sharing them harmless today and makes cloning them
	// free. They are here because the rule is the struct field and not the
	// current set of writers: a keyed write added to either one later is a
	// change nobody would think to look at this file for.
	c.freezing = maps.Clone(r.freezing)
	c.declaring = maps.Clone(r.declaring)

	// The option table, on the same terms: `(setopt …)` is the subshell's.
	c.extraOptions = maps.Clone(r.extraOptions)

	// The descriptor tables are copied and the streams in them are shared: a
	// subshell's `exec 7>&1` must not appear in the parent, and its writes
	// through a descriptor the parent made must still land where the parent
	// pointed it. execFds is the record of which of them `exec` marked, so
	// it travels with fds or it describes the wrong table.
	c.fds = maps.Clone(r.fds)
	c.execFds = maps.Clone(r.execFds)

	c.completions = maps.Clone(r.completions)

	// The tables that say what a command *name* means, on the same terms as
	// the variable tables above: a subshell inherits them and owns what it
	// then does to them. All four shells in the panel agree, and they agree
	// in both directions — `(g(){ :; }); type g` finds nothing afterwards,
	// and `f(){ :; }; (unset -f f); type f` still finds `f`. Sharing the maps
	// made a definition made in a subshell the parent's, and a removal made
	// in one the parent's too, at status 0 with nothing said either way.
	c.funcs = maps.Clone(r.funcs)
	c.funcFiles = maps.Clone(r.funcFiles)
	c.exportedFuncs = maps.Clone(r.exportedFuncs)
	c.aliases = maps.Clone(r.aliases)
	c.disabledBuiltins = maps.Clone(r.disabledBuiltins)

	// The extension points. An embedder registers these before a run and a
	// dialect's Apply is the usual caller, so nothing a *script* does writes
	// one — but the shell itself does: Dynamic is built lazily, and `_` and
	// `LINENO` are installed into it on the way past. That write is on the
	// same goroutine question as every table above, and a substitution
	// looking up a variable is enough to make it.
	c.Dynamic = maps.Clone(r.Dynamic)
	c.DynamicArrays = maps.Clone(r.DynamicArrays)
	c.DynamicAssocs = maps.Clone(r.DynamicAssocs)
	c.dynamicAssocWriters = maps.Clone(r.dynamicAssocWriters)
	// absentElements travels with DynamicAssocs and its writer table, and
	// copying two of the three would be the split dynamicWriters describes
	// below: a subshell owning the producer while sharing the sentence a key
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
