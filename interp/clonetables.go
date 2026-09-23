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
		c.Arrays[k] = v.clone()
	}
	c.AssocArrays = make(map[string]AssocArray, len(r.AssocArrays))
	for k, v := range r.AssocArrays {
		c.AssocArrays[k] = v.clone()
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
	//	declaredOnlyCompound
	//	               an array or table a declaration made and nothing has
	//	               written to, which is not the same as an emptied one
	//	declaredBare   the scalar of that: a name a declaration made with no
	//	               letters and no value, which the listing writes and
	//	               nothing else can see
	//	compoundVariable
	//	               ksh93's fourth kind: the name is a compound variable,
	//	               so what a bare `$c` reads and what `typeset -p` writes
	//	               come from the members stored under it
	c.removed = maps.Clone(r.removed)
	c.assigned = maps.Clone(r.assigned)
	c.absentParams = maps.Clone(r.absentParams)
	c.declaredEmpty = maps.Clone(r.declaredEmpty)
	c.declaredOnlyCompound = maps.Clone(r.declaredOnlyCompound)
	c.compoundHeldAnElement = maps.Clone(r.compoundHeldAnElement)
	c.declaredBare = maps.Clone(r.declaredBare)
	c.unsetLeftItDeclared = maps.Clone(r.unsetLeftItDeclared)
	c.compoundVariable = maps.Clone(r.compoundVariable)
	// The namespaces a `namespace NAME { … }` block has declared. A subshell
	// that opens one must not leave it behind: measured, `( namespace ns {
	// x=1; } )` then `${.ns.x}` is unset. The members go with it, since they
	// are ordinary names in the tables above.
	c.namespaces = maps.Clone(r.namespaces)

	// The attribute tables `declare` and `typeset` write. A subshell's
	// attribute must not outlive it: measured, `x=1; (readonly x); x=2`
	// assigns 2 in bash, ksh93 and zsh, and `n=5; (typeset -i n); n=1+1`
	// leaves the three characters everywhere.
	// The `set -o` names this shell remembers and does not act on. A
	// subshell's `set -o markdirs` must not reach the parent, exactly as an
	// implemented option's would not: the states behind those are plain
	// fields and are copied by `c := *r`, and this is the table that has to
	// be cloned to get the same answer.
	c.recordedOptions = maps.Clone(r.recordedOptions)

	c.readonly = maps.Clone(r.readonly)
	c.integer = maps.Clone(r.integer)
	c.integerBase = maps.Clone(r.integerBase)
	c.floatPrecision = maps.Clone(r.floatPrecision)
	c.floatExponent = maps.Clone(r.floatExponent)
	c.fieldWidth = maps.Clone(r.fieldWidth)
	c.lowered = maps.Clone(r.lowered)
	c.uppered = maps.Clone(r.uppered)
	c.hidden = maps.Clone(r.hidden)
	c.localMarked = maps.Clone(r.localMarked)
	c.unique = maps.Clone(r.unique)
	c.traced = maps.Clone(r.traced)
	c.nameref = maps.Clone(r.nameref)
	c.hideInScope = maps.Clone(r.hideInScope)
	c.tied = maps.Clone(r.tied)
	// The type names `typeset -T` registered — see interp/declaretype.go.
	// A slice rather than a map, so it is copied outright: a subshell that
	// declares a type must not append into the array its parent is holding.
	c.declaredTypes = slices.Clone(r.declaredTypes)
	// freezing, literalOperands and declaring are only ever *replaced* as a
	// whole, which makes sharing them harmless today and makes cloning them
	// free. They are here because the rule is the struct field and not the
	// current set of writers: a keyed write added to one of them later is a
	// change nobody would think to look at this file for.
	c.freezing = maps.Clone(r.freezing)
	c.literalOperands = maps.Clone(r.literalOperands)
	c.compoundOperands = maps.Clone(r.compoundOperands)
	c.compoundOperandUnset = maps.Clone(r.compoundOperandUnset)
	// indexedLetterHere *is* written by key, one name at a time, so it is
	// here on the stronger footing than the three above it.
	c.indexedLetterHere = maps.Clone(r.indexedLetterHere)
	c.tableLetterHere = maps.Clone(r.tableLetterHere)
	c.globalLetterHere = maps.Clone(r.globalLetterHere)
	c.declaring = maps.Clone(r.declaring)
	c.precommands = maps.Clone(r.precommands)
	c.conditionAnswers = maps.Clone(r.conditionAnswers)

	// The option table, on the same terms: `(setopt …)` is the subshell's.
	c.extraOptions = maps.Clone(r.extraOptions)
	c.negatedOptions = maps.Clone(r.negatedOptions)
	c.immovableOptions = maps.Clone(r.immovableOptions)
	c.inertOptions = maps.Clone(r.inertOptions)
	c.defaultOnOptions = maps.Clone(r.defaultOnOptions)

	// The descriptor tables are copied and the streams in them are shared: a
	// subshell's `exec 7>&1` must not appear in the parent, and its writes
	// through a descriptor the parent made must still land where the parent
	// pointed it. execFds is the record of which of them `exec` marked, so
	// it travels with fds or it describes the wrong table.
	c.fds = maps.Clone(r.fds)
	c.execFds = maps.Clone(r.execFds)
	// The commands started beside the shell under a name, on the terms the
	// jobs slice is on and for a measured reason rather than a symmetrical
	// one: the *table* is the subshell's and the commands in it are shared.
	// A subshell that deletes one reaches the running command and forgets
	// only its own copy of the name, which is what the four rows at the top
	// of concurrentcommand.go record. Sharing the map would take the name out
	// of the parent too; copying the values would leave the command running.
	c.concurrent = maps.Clone(r.concurrent)
	// And on the same terms, for the same reason: a subshell's `sysopen -o
	// cloexec` must not decide what the parent hands to a child.
	c.cloexecFds = maps.Clone(r.cloexecFds)

	// Deep, unlike the maps above: a spec holds two slices, and a subshell's
	// `compopt -o nospace f` writing into a backing array the parent still
	// points at would change the parent's spec from inside a subshell — the
	// one thing a subshell may never do.
	if r.completions != nil {
		c.completions = make(map[string]completionSpec, len(r.completions))
		for name, spec := range r.completions {
			c.completions[name] = completionSpec{
				options: slices.Clone(spec.options),
				words:   slices.Clone(spec.words),
			}
		}
	}

	// The tables that say what a command *name* means, on the same terms as
	// the variable tables above: a subshell inherits them and owns what it
	// then does to them. All four shells in the panel agree, and they agree
	// in both directions — `(g(){ :; }); type g` finds nothing afterwards,
	// and `f(){ :; }; (unset -f f); type f` still finds `f`. Sharing the maps
	// made a definition made in a subshell the parent's, and a removal made
	// in one the parent's too, at status 0 with nothing said either way.
	c.funcs = maps.Clone(r.funcs)
	// And which variables have a discipline function watching them, which is
	// a view of that same table and goes with it: measured on ksh93u+,
	// `g=raw; ( function g.get { .sh.value=sub; }; echo "$g" )` answers `sub`
	// inside and `raw` after, so a hook defined in a subshell is not the
	// parent's. Sharing the map would have made the parent's next read of
	// `g` build a name and ask funcs for a function the subshell took with
	// it — harmless today and exactly the shape this file exists to prevent.
	c.disciplined = maps.Clone(r.disciplined)
	// The re-entry guard goes with it for a plainer reason: a subshell
	// started from inside a hook is still inside it, so it must not fire the
	// hook it is running, and a subshell that finishes must not leave the
	// parent's guard set. A copy is both.
	c.disciplineRunning = maps.Clone(r.disciplineRunning)
	// And which of those functions stand for a trapped condition, on the same
	// terms: a `TRAPZERR` defined inside a subshell is not the parent's
	// handler afterwards. inheritTraps prunes what the subshell does not
	// keep — see trapfunction.go.
	c.trapFuncs = maps.Clone(r.trapFuncs)
	// And the math-function registrations, which are the same kind of table
	// under a second name: a `functions -M` made inside a subshell is not
	// the parent's afterwards, and one the parent made is the subshell's to
	// call. mathOrder is a slice and is copied outright, so a removal in the
	// subshell cannot shorten the parent's.
	c.mathFuncs = maps.Clone(r.mathFuncs)
	c.mathOrder = append([]string(nil), r.mathOrder...)
	c.funcOrigins = maps.Clone(r.funcOrigins)
	c.exportedFuncs = maps.Clone(r.exportedFuncs)
	// And the freeze, for the same reason: a subshell that froze a function
	// has not frozen the parent's, and one the parent froze is frozen in
	// there — measured, `readonly -f f; ( f() { :; } )` is refused inside the
	// subshell in bash 5.3.20.
	c.readonlyFuncs = maps.Clone(r.readonlyFuncs)
	c.tracedFuncs = maps.Clone(r.tracedFuncs)
	c.aliases = maps.Clone(r.aliases)
	c.suffixAliases = maps.Clone(r.suffixAliases)
	// And the names one dialect remembers having named, which is owned
	// rather than shared *and that is a known gap rather than the answer*:
	// real ksh93 lets a name a `( … )` looked up satisfy the parent's later
	// `unalias`, which needs one set the two write. Sharing a map across a
	// clone is what the paragraph above this file's first function is about
	// — a process substitution is a clone on a goroutine — so the sharing
	// half waits for a set that can be shared safely (#3429). What is
	// reproduced is that no remembered name counts inside a subshell, which
	// is asked at the `unalias` rather than here.
	c.namedAliases = maps.Clone(r.namedAliases)
	c.markedAliases = maps.Clone(r.markedAliases)
	c.unreportedAliases = maps.Clone(r.unreportedAliases)
	// And the command hash, which is the same kind of table under a third
	// name: what PATH last resolved a name to. A subshell owns its entries —
	// measured, `(ls >/dev/null); hash` leaves the parent's table empty in
	// bash, zsh and dash — and the order slice is copied outright so a
	// forgetting inside one cannot shorten the parent's listing.
	c.cmdHash = maps.Clone(r.cmdHash)
	c.cmdHashOrder = append([]string(nil), r.cmdHashOrder...)
	// And the named directories beside it, for the same reason: `hash -d`
	// inside a subshell is that subshell's, exactly as `hash` is.
	c.namedDirs = maps.Clone(r.namedDirs)
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
	// And the names among them `unset` may not take away, which a dialect
	// registers beside the producer.
	c.unsetRefused = maps.Clone(r.unsetRefused)
	c.DynamicAssocs = maps.Clone(r.DynamicAssocs)
	c.dynamicAssocElements = maps.Clone(r.dynamicAssocElements)
	c.dynamicAssocWriters = maps.Clone(r.dynamicAssocWriters)
	// And the emptying policy travels with the writer it is expressed
	// through: a subshell that owned the writer while sharing this would
	// decide by the parent's table which of its own writes clear first.
	c.dynamicAssocEmptied = maps.Clone(r.dynamicAssocEmptied)
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
	// And the same for a stored name whose assignment does something: a
	// subshell registering one of its own must not put an action on the
	// parent's name, which is the split every table above this one avoids.
	c.assignmentActions = maps.Clone(r.assignmentActions)
	// And the removal half of the same message, for the same reason.
	c.unsetActions = maps.Clone(r.unsetActions)
	// And the startup half. A subshell never delivers these — the shell was
	// launched once — but the table is cloned for the same reason the two
	// above it are: a subshell registering a name must not reach the parent's.
	c.inheritedParameterActions = maps.Clone(r.inheritedParameterActions)
	// And the names restricted mode froze. A subshell of a restricted shell is
	// restricted — measured, `( cd / )` is the refusal in both columns — so the
	// record is carried; cloned rather than shared because `set +r` in the
	// subshell empties it in the one dialect that grants that, and the parent
	// must keep what it froze.
	c.restrictedFrozen = maps.Clone(r.restrictedFrozen)
	// And the option half of a tie between a `set -o` name and a parameter,
	// which is the same message from the other side. See
	// interp/tiedoption.go.
	c.optionTies = maps.Clone(r.optionTies)
	c.dynamicPresence = maps.Clone(r.dynamicPresence)
	// How each produced parameter lists back travels with the producer it
	// describes, for the same reason: a subshell that registers one of its
	// own must not put a row into the parent's listings.
	c.dynamicDeclarations = maps.Clone(r.dynamicDeclarations)
	c.rejoinedOperands = maps.Clone(r.rejoinedOperands)
	c.producedReading = maps.Clone(r.producedReading)
	// endedProducers travels with them for the third time and the same
	// reason: it is the record that keeps a producer from being registered
	// again, so a subshell that ends one while sharing this table would end
	// the parent's parameter too.
	c.endedProducers = maps.Clone(r.endedProducers)
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
	// And the record beside it, for the same reason and with the same shape:
	// a call pushes into it and its return pops, so a subshell started inside
	// a call would share the array and both would write [len-1]. Measured to
	// matter the moment two clones each enter a function, which is `f | f`
	// under extended debugging.
	c.callArgs = slices.Clone(r.callArgs)
	// The same reason, for the same shape: `runSourced` appends to this and
	// truncates it back, so a subshell started from inside a sourced file
	// would share the array and both would write [len-1].
	c.borrowed = slices.Clone(r.borrowed)
	// The prefix values `set -x` expanded for the command that is running
	// right now. A clone is started *during* one — a command substitution in
	// a prefix's own value is the case — and the command it goes on to run is
	// not that one, so it inherits none of them. Cloned rather than left
	// aliased for the reason the reaped jobs are: "the next command clears
	// it" is a fact about the caller, and an append into an array the parent
	// still holds is what this file exists to prevent.
	c.prefixTraceAssigns = slices.Clone(r.prefixTraceAssigns)
	c.prefixTraceValues = slices.Clone(r.prefixTraceValues)
	// And where that command's declaration operands stood, which a clone
	// inherits for exactly as long as it takes to run a command of its own —
	// the same reading, and cloned for the same reason.
	c.declarationOperands = slices.Clone(r.declarationOperands)
	// And its array-literal operands, for the same reason and with the same
	// reading — a clone appending into the parent's slice is exactly what this
	// file exists to prevent.
	c.arrayOperands = slices.Clone(r.arrayOperands)
	// And which of that command's operands were written with their brackets
	// unquoted, for the same reason and with the same reading. See
	// Runner.lexedSubscriptOperands.
	c.lexedSubscriptOperands = slices.Clone(r.lexedSubscriptOperands)
	// And the names the running command's prefix is holding, with what they
	// held before it and what a declaration has done to them, for the same
	// reason and with the same reading: a clone started inside a prefixed
	// command goes on to run a different command, and appending into an array
	// the parent still holds is the hazard this file is about. See
	// Runner.prefixHeldNames.
	c.prefixHeldNames = slices.Clone(r.prefixHeldNames)
	c.prefixKeptNames = slices.Clone(r.prefixKeptNames)
	c.prefixShadowed = slices.Clone(r.prefixShadowed)
	// And the names a `-g` declaration has lifted the running command's own
	// prefix off, which is per-command scratch on the same footing: see
	// Runner.globalUnderItsOwnPrefix.
	c.globalUnderItsOwnPrefix = slices.Clone(r.globalUnderItsOwnPrefix)
	c.prefixHeldUndo = slices.Clone(r.prefixHeldUndo)
	c.functionPrefixNames = slices.Clone(r.functionPrefixNames)
	// A frame at a time, because a subshell may take a name out of one and
	// the parent's frame must not lose it: the slice of frames is cloned and
	// so is each frame's own pair of slices.
	c.callPrefixes = cloneCallPrefixes(r.callPrefixes)
	c.scopes = cloneScopes(r.scopes)
	// Appended to in place as well, so each needs an array of its own. Their
	// *elements* stay shared on purpose: a `*Job` is one job to whoever holds
	// it, and copying the pointer is what keeps a subshell looking at the
	// parent's job rather than a snapshot of it.
	c.redirFds = slices.Clone(r.redirFds)
	// The chain of names an arithmetic value is being resolved through is
	// pushed and popped around each resolution, so a clone evaluating its own
	// arithmetic must not write into the parent's array — the cycle bound
	// reads it back to decide whether a name has come round on itself.
	c.arithValueNames = slices.Clone(r.arithValueNames)
	// And the numbers a substitution's body is entitled to re-use, which
	// substRunner replaces wholesale a moment later for the body itself —
	// cloned here anyway, for the reason reaped is below: "the next thing
	// that runs replaces it" is a fact about the caller and not about the
	// field, and every append to this one lands on the substitution path
	// where the clone runs on a goroutine of its own.
	c.releasedSubstFds = slices.Clone(r.releasedSubstFds)
	c.jobs = slices.Clone(r.jobs)
	c.jobOrder = slices.Clone(r.jobOrder)
	// And the memory of the ones already reported, which a body a real shell
	// would have forked does not inherit at all — inheritJobs empties it a
	// moment after this. It is cloned here anyway rather than left aliased,
	// because "the next thing that runs clears it" is a fact about the caller
	// and not about the field: `reap` appends, and an append into an array
	// the parent still holds is the shape this whole file exists to prevent.
	c.reaped = slices.Clone(r.reaped)
	c.procSubJobs = slices.Clone(r.procSubJobs)
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
	// Where each of the originals sits, so that a sealed name's holder — a
	// pointer *into this same stack* — can be repointed at the clone's copy
	// of it. Left as it was, a subshell's return would write the shell's own
	// name back into the parent's scope, which is the very sharing this
	// function exists to end. See staticscope.go.
	at := make(map[*scope]int, len(scopes))
	for i, sc := range scopes {
		at[sc] = i
	}
	for i, sc := range scopes {
		c := *sc
		c.saved = maps.Clone(sc.saved)
		c.existed = maps.Clone(sc.existed)
		c.arrayExisted = maps.Clone(sc.arrayExisted)
		c.removedBefore = maps.Clone(sc.removedBefore)
		c.memberNamespaces = maps.Clone(sc.memberNamespaces)
		c.compoundMarkBefore = maps.Clone(sc.compoundMarkBefore)
		c.namespaceOwned = maps.Clone(sc.namespaceOwned)
		c.declaredOnlyBefore = maps.Clone(sc.declaredOnlyBefore)
		c.heldAnElementBefore = maps.Clone(sc.heldAnElementBefore)
		c.assocExisted = maps.Clone(sc.assocExisted)
		c.savedReadonly = maps.Clone(sc.savedReadonly)
		c.savedHideInScope = maps.Clone(sc.savedHideInScope)
		c.hiddenShadow = maps.Clone(sc.hiddenShadow)
		c.suspendedProducers = maps.Clone(sc.suspendedProducers)
		c.savedAttrs = maps.Clone(sc.savedAttrs)
		c.savedAssigned = maps.Clone(sc.savedAssigned)
		c.assignedSpoken = maps.Clone(sc.assignedSpoken)
		c.savedExported = maps.Clone(sc.savedExported)
		c.exportedSpoken = maps.Clone(sc.exportedSpoken)
		c.exportedShadow = maps.Clone(sc.exportedShadow)
		c.savedTraps = maps.Clone(sc.savedTraps)
		c.savedOptions = maps.Clone(sc.savedOptions)
		c.sealed = cloneSealed(sc.sealed, at, out)
		// The two that hold a container per name, on the same terms as Arrays
		// and AssocArrays above: cloning the outer map alone would give the
		// clone its own name table pointing at the parent's elements.
		if sc.savedArrays != nil {
			c.savedArrays = make(map[string]Array, len(sc.savedArrays))
			for k, v := range sc.savedArrays {
				c.savedArrays[k] = v.clone()
			}
		}
		if sc.savedAssoc != nil {
			c.savedAssoc = make(map[string]AssocArray, len(sc.savedAssoc))
			for k, v := range sc.savedAssoc {
				c.savedAssoc[k] = v.clone()
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

// cloneSealed copies a scope's sealed names, giving the clone its own copy of
// every container a binding holds and repointing each holder at the clone's
// own scope.
//
// The holder is looked up by position rather than carried across, for the
// reason the whole file exists: it is a pointer into the stack being cloned,
// and a clone that kept it would hand a subshell the parent's scope to write.
// A holder the stack does not contain cannot happen — a seal only ever names
// a scope below it in the same stack — and is left alone rather than guessed
// at, since an index that is not there has no clone to point at.
func cloneSealed(sealed map[string]sealedName, at map[*scope]int, out []*scope) map[string]sealedName {
	if sealed == nil {
		return nil
	}
	c := make(map[string]sealedName, len(sealed))
	for name, sn := range sealed {
		sn.held.array = sn.held.array.clone()
		sn.held.assoc = sn.held.assoc.clone()
		if i, ok := at[sn.holder]; ok {
			sn.holder = out[i]
		}
		c[name] = sn
	}
	return c
}
