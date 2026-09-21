// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "slices"

// An assignment prefix in front of a builtin is two things at once, and the
// panel does not agree about which: the *environment* the builtin is handed,
// or a value this shell holds for the length of one command. The two readings
// part in two observable places, and this file is both of them.
//
// The first is the export attribute — Semantics.PrefixExportAtABuiltin — which
// decides what `c=2 declare -p c` reads back and what a child of `v=9 eval
// env` is told.
//
// The second is what a *declaration* inside that command does to the entry.
// Where the builtin names the export or the readonly attribute over the very
// name the prefix is standing in front of, bash keeps the prefix's value
// instead of giving it back, so `b=7; b=8 readonly b` leaves `8` frozen and
// exported. See Semantics.DeclarationPromotesThePrefixEntry, and #3437 for
// the measurement that separates it from the special-builtin persistence rule
// it is so easily mistaken for.

// prefixExportAtABuiltin says whether the export attribute moves for the
// length of a builtin its prefix stands in front of, and which way.
//
// moves is false for the column that leaves the attribute where it was, which
// is a third answer rather than either direction — see
// PrefixExportAtABuiltinPolicy. An unanswered axis is refused by name here
// like any other, and reports moves false so the caller writes nothing on top
// of the refusal.
//
// Asked on a *persisting* prefix too, which is measured rather than assumed:
// the obvious narrowing is that a prefix the shell keeps is an ordinary
// assignment and an ordinary assignment exports nothing, and bash says
// otherwise. Under `set -o posix` — where AssignmentPrefixPersistsOnSpecialBuiltin
// turns Yes — `b=7; b=8 :` leaves `declare -x b="8"` and `v=9 eval env` still
// hands the child `v=9`. Measured 2026-09-16 in 5.3.20; the narrowing cost
// three lines of bash's own varenv suite before it was taken back out (#3437).
func (r *Runner) prefixExportAtABuiltin() (on, moves bool) {
	switch r.sem().PrefixExportAtABuiltin {
	case PrefixExportAtABuiltinOn:
		return true, true
	case PrefixExportAtABuiltinOff:
		return false, true
	case PrefixExportAtABuiltinUnchanged:
		return false, false
	}
	r.diagf("%s\n", r.unanswered("an assignment prefix moving the export attribute at a builtin"))
	r.status = 2
	r.unspecified = true
	return false, false
}

// keepThePrefixEntry is a declaration saying that the name it has just given
// the export or the readonly attribute to is one the shell keeps, rather than
// one the running command's prefix gives back when it ends.
//
// Called from the two places an attribute is put on — markReadonly and
// declarationExports — rather than from each builtin that can name one,
// because `readonly x`, `export x`, `typeset -r x`, `local -x x` and
// `declare -ir x` are five spellings of the same event and the measurement
// does not tell them apart: it is the attribute that keeps the value, not the
// word.
//
// Asked at the disagreement and nowhere else. A name the running command's
// prefix is not holding cannot be kept and asks nothing, which is every
// `readonly` and `export` a script writes on its own line.
func (r *Runner) keepThePrefixEntry(name string) {
	if !slices.Contains(r.prefixHeldNames, name) {
		return
	}
	if slices.Contains(r.globalUnderItsOwnPrefix, name) {
		// A `-g` declaration, which is writing the cell **underneath** this
		// prefix rather than the binding the prefix made — so there is no
		// prefix value here for it to keep. The letter takes the question
		// away rather than answering it: measured 2026-09-18 on bash 5.3.20,
		// `a=7 declare -x a` leaves `declare -x a="7"` and `t=7 declare -gx
		// t` leaves `declare -x t`, the attribute with no value at all, and
		// `c=7 declare -r c` leaves `declare -rx c="7"` against `b=7 declare
		// -gr b=3` leaving `declare -r b="3"` — without even the export the
		// prefix put on the temporary. See
		// interp/globalunderitsownprefix.go.
		return
	}
	if slices.Contains(r.prefixKeptNames, name) {
		// Two attributes over one name — `declare -rx v` — is one keeping.
		return
	}
	if !r.ask(r.sem().DeclarationPromotesThePrefixEntry,
		"a declaration keeping the value its own assignment prefix set") {
		return
	}
	r.prefixKeptNames = append(r.prefixKeptNames, name)
	// And what is kept is the prefix's *value*, written into the binding the
	// prefix displaced through the ordinary rules — not the fresh cell the
	// command was shown. Here rather than at the take-back because the
	// caller is about to record an attribute that would refuse the write.
	// See Runner.prefixEntryTakesTheDisplacedShapeBack.
	r.prefixEntryTakesTheDisplacedShapeBack(name)
}

// prefixEntryShadowed records that a declaration has taken a fresh scope for a
// name a live assignment prefix is holding — the running builtin's own, or the
// running function call's.
//
// Two things follow, and they are the two halves of one fact: the entry has
// moved into the cell the declaration made.
//
// The cell keeps the value, because what it displaced *is* the prefix's — see
// prefixEntryIsInThisCell, read by declareEmpty.
//
// And the outer name is no longer this command's to keep, so it comes off
// prefixHeldNames: `b=8; f(){ b=4 declare -r b; }` freezes the local at `4`
// and gives the caller its `8` back unfrozen, where keeping the outer entry
// would have left the *shell* holding `4`. Measured 2026-09-16 in bash 5.3.20
// (#3437).
//
// The savedVar comes back for a name the *running builtin's* prefix was
// holding, because that prefix ends before the scope does and the scope has to
// give back what it would have given back — see scopeTakesOverThePrefixEntry. A
// function call's prefix is taken back after its own scope has already popped,
// so there is nothing to hand over there and ok is false.
func (r *Runner) prefixEntryShadowed(name string) (savedVar, bool) {
	held := slices.Contains(r.prefixHeldNames, name)
	if !held && !slices.Contains(r.functionPrefixNames, name) {
		return savedVar{}, false
	}
	// The cell about to be saved is the scope's to give back, so the fresh
	// cell the prefix made has to have gone by now: a scope that saved a
	// plain scalar would hand the shell one on return where its array had
	// been. Ahead of everything the caller saves, which is why this stands
	// in the first lines of shadow. See
	// Runner.prefixEntryTakesTheDisplacedShapeBack.
	r.prefixEntryTakesTheDisplacedShapeBack(name)
	if !slices.Contains(r.prefixShadowed, name) {
		r.prefixShadowed = append(r.prefixShadowed, name)
	}
	if !held {
		return savedVar{}, false
	}
	r.prefixHeldNames = slices.DeleteFunc(slices.Clone(r.prefixHeldNames),
		func(n string) bool { return n == name })
	for _, u := range r.prefixHeldUndo {
		if u.name == name {
			return u, true
		}
	}
	return savedVar{}, false
}

// scopeTakesOverThePrefixEntry hands a shadowed prefix entry's take-back from
// the command to the scope.
//
// The two orderings are what makes this necessary. A *function* call's prefix
// is given back after the body's scope has popped, so the scope restoring the
// prefix's own value and the call then restoring the caller's comes out right.
// A *builtin's* prefix is given back when the builtin ends, which is while the
// scope its declaration made is still open — so the command's restore would
// write over the local it had just declared, and the scope's would then put the
// prefix's value back for good. `b=8; f(){ b=4 declare b; }; f` left the shell
// holding `4` where bash 5.3.20 leaves `8`.
//
// So the command stops restoring the name — restoreVarsExcept skips what
// prefixShadowed names — and the scope gives back the state from before the
// prefix instead. Only the four things a prefix moves: the value, whether the
// name existed, whether `unset` had hidden it, and the export attribute. Every
// other entry the shadow recorded is the outer name's own and is left alone.
func (r *Runner) scopeTakesOverThePrefixEntry(sc *scope, name string, u savedVar) {
	sc.saved[name] = u.value
	sc.existed[name] = u.present
	if sc.removedBefore != nil {
		sc.removedBefore[name] = u.removed
	}
	if sc.exportedSpoken != nil {
		sc.exportedSpoken[name] = u.exportSpoken
		sc.savedExported[name] = u.exported
	}
}

// prefixEntryIsInThisCell reports whether the fresh cell a declaration has just
// made is holding an assignment prefix's value rather than nothing.
func (r *Runner) prefixEntryIsInThisCell(name string) bool {
	return slices.Contains(r.prefixShadowed, name)
}

// declarationExports records the export attribute a declaration is putting on
// or taking off a name, and lets the keeping above hear about it.
//
// Only the putting-on keeps: `t=1; t=2 typeset +x t` reads `declare -- t="1"`
// in bash, so a plus form that takes the attribute away leaves the shell's own
// value exactly where it was.
func (r *Runner) declarationExports(name string, on bool) {
	if r.exported == nil {
		r.exported = map[string]bool{}
	}
	r.exported[name] = on
	if on {
		r.keepThePrefixEntry(name)
	}
}

// restoreVarsExcept is restoreVars with two sets of names left alone.
//
// kept is what a declaration took for the shell — value, attributes and all,
// since what the declaration wrote is exactly what it kept.
//
// shadowed is what a declaration took into a scope of its own, where restoring
// would write over the cell it had just made; that take-back has been handed to
// the scope instead. See scopeTakesOverThePrefixEntry.
func (r *Runner) restoreVarsExcept(undo []savedVar, kept, shadowed []string) {
	if len(kept) == 0 && len(shadowed) == 0 {
		r.restoreVars(undo)
		return
	}
	for i := len(undo) - 1; i >= 0; i-- {
		if slices.Contains(kept, undo[i].name) || slices.Contains(shadowed, undo[i].name) {
			continue
		}
		r.restoreVar(undo[i])
	}
}

// exportTheArrayOfAnElement records the export letter on the **array** where
// `export a[1]` and `export a[1]=v` record one at all.
//
// One column does and puts the value in a child's environment; the other
// writes the element and leaves the array's attributes alone, so the listing
// there shows no `x` at all. Both operand shapes go through this one call, so
// a dialect cannot come to answer the valueless spelling and the one with a
// value differently. See
// Semantics.ExportThroughASubscriptedOperandRecordsTheLetter for the rows.
func (r *Runner) exportTheArrayOfAnElement(base string, on bool) {
	if r.ask(r.sem().ExportThroughASubscriptedOperandRecordsTheLetter,
		"an `export` through a subscripted operand recording the letter on the array") {
		r.declarationExports(base, on)
	}
}

// callPrefixesLetTheNameGo is what a prefix the shell **keeps** does to the
// frames an enclosing call's prefix is holding: the name leaves every one of
// them, and what they had displaced is not given back.
//
// Two prefixes are live over one name at once, and the inner one has just
// become the shell's. Without this the outer one's take-back runs over it on
// the way out and the name comes back to whatever it was before the call —
// which is the whole of #3447.
//
// Measured 2026-09-18 on bash 5.3.20, `env -i PATH=/usr/bin:/bin LC_ALL=C`
// with a scratch HOME, from a script file, under `set -o posix` so that a
// prefix in front of a special builtin persists:
//
//	f(){ e=3 readonly e; }; e=7 f; echo "[${e-U}]"        [3]
//	f(){ m=3 :; };         m=7 f; echo "[${m-U}]"         [3]
//	inner(){ n=3 readonly n; }
//	outer(){ n=7 inner; echo "[${n-U}]"; }
//	n=9 outer; echo "[${n-U}]"                            [3] and [3]
//
// The second row is the one that says which mechanism this is: **no
// declaration is written at all**, so it is not the declaration's keeping
// (Semantics.DeclarationPromotesThePrefixEntry) that survives the call — it
// is the persistence itself. The issue named the declaration, and the row
// that separates them is the one with nothing but a colon in it.
//
// And the third is why every frame gives the name up rather than the
// innermost: both enclosing calls read 3 afterwards, so a drop that stopped
// at the nearest frame would have let the outer one put 9 back at the top.
//
// The controls, which are what keep this from being "a write wins": with the
// same enclosing prefix, a plain `v=3`, an `export w=3`, a `readonly x=3` and
// a `y=3 export y` all leave the name **unset** after the call, in `posix`
// mode and out of it. A prefix that does not persist is given back exactly as
// it always was.
func (r *Runner) callPrefixesLetTheNameGo(name string) {
	for i := range r.callPrefixes {
		f := &r.callPrefixes[i]
		if !slices.Contains(f.names, name) {
			continue
		}
		f.names = slices.DeleteFunc(slices.Clone(f.names),
			func(n string) bool { return n == name })
		// The undo entry goes with it and is **not** replayed, which is the
		// difference between this and the `unset` spelling next door: that
		// one gives the displaced value back because the name is meant to
		// disappear, and here the name is meant to stand at what the inner
		// prefix left in it.
		if j := slices.IndexFunc(f.undo, func(u savedVar) bool { return u.name == name }); j >= 0 {
			f.undo = slices.Delete(slices.Clone(f.undo), j, j+1)
		}
	}
}
